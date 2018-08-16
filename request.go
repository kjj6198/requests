package requests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	MethodGet    = "GET"
	MethodPatch  = "PATCH"
	MethodPut    = "PUT"
	MethodOption = "OPTION"
	MethodPost   = "POST"
)

const (
	ChromeAgent    = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/51.0.2704.103 Safari/537.36"
	FirefoxAgent   = "Mozilla/5.0 (Macintosh; Intel Mac OS X x.y; rv:42.0) Gecko/20100101 Firefox/42.0"
	SafariAgent    = "Mozilla/5.0 (Linux; U; Android 4.0.3; de-ch; HTC Sensation Build/IML74K) AppleWebKit/534.30 (KHTML, like Gecko) Version/4.0 Mobile Safari/534.30"
	IEAgent        = "Mozilla/5.0 (compatible; MSIE 9.0; Windows Phone OS 7.5; Trident/5.0; IEMobile/9.0)"
	GoogleBotAgent = "Googlebot/2.1 (+http://www.google.com/bot.html)"
)

type safeClient struct {
	client *http.Client
	mutex  *sync.RWMutex
}

var defaultTransport = &http.Transport{
	Dial: (&net.Dialer{
		KeepAlive: 120 * time.Second,
		Timeout:   100 * time.Second, // TCP connection timeout
	}).Dial,
	TLSHandshakeTimeout:   100 * time.Second,
	ResponseHeaderTimeout: 100 * time.Second,
	IdleConnTimeout:       100 * time.Second,
	MaxIdleConns:          100,
	MaxIdleConnsPerHost:   100,
}

var client = &http.Client{
	Transport: defaultTransport,
	Timeout:   100 * time.Second, // default to 15 second to timeout a request
}

type Config struct {
	Headers      map[string]string
	Params       url.Values
	URL          string
	Method       string
	ResponseType string
	IsBot        bool
	Body         map[string]interface{}
	Timeout      time.Duration
	TimeoutChan  chan *http.Request
}

func checkConfig(config Config) error {
	if config.URL == "" || config.Method == "" {
		return errors.New("URL and method can not be nil")
	}

	return nil
}

func setHeaders(c *Config, req *http.Request) error {
	if c.Headers == nil {
		return fmt.Errorf("header is nil")
	}

	for k, v := range c.Headers {
		req.Header.Add(k, v)
	}

	return nil
}

func handleTimeout(ctx context.Context, req *http.Request, reqCh chan *http.Request) {
	select {
	case <-ctx.Done():
		log.Println("DONE REQUEST")
		return
	default:
		if reqCh != nil {
			reqCh <- req
		}
		return
	}
}

func Request(ctx context.Context, config Config) (*http.Response, string, error) {
	err := checkConfig(config)
	if err != nil {
		panic(err)
	}

	if ctx != nil {
		ctx, _ = context.WithCancel(context.Background())
	}

	request := &http.Request{
		Header: make(http.Header),
	}

	if ctx == nil {
		panic("ctx param can not be nil")
	}

	request = request.WithContext(ctx)

	setHeaders(&config, request)
	if !config.IsBot {
		request.Header.Set("User-Agent", ChromeAgent)
	}

	reqURL, _ := url.Parse(config.URL)

	request.URL = reqURL
	request.Method = config.Method
	if request.Method == MethodPost {
		contentType := request.Header.Get("Content-Type")
		switch contentType {
		case "x-www-form-urlencoded":
			for k, v := range config.Body {
				request.Form.Add(k, v.(string))
			}
		case "application/json":
			data, _ := json.Marshal(config.Body)
			request.Body = ioutil.NopCloser(bytes.NewReader(data))
		}
	}

	if request.Method == MethodGet {
		if config.Params != nil {
			values := &url.Values{}
			for k, v := range config.Params {
				for _, val := range v {
					values.Set(k, val)
				}
			}
			encodedURL, _ := url.Parse(fmt.Sprintf("%s?%s", config.URL, values.Encode()))
			request.URL = encodedURL
		}
	}

	client.Timeout = config.Timeout
	resp, err := client.Do(request)

	// TODO: feels like redundant...
	go handleTimeout(ctx, request, config.TimeoutChan)

	if err != nil {
		return nil, "", err
	}

	respData, err := ioutil.ReadAll(resp.Body)

	return resp, string(respData), err
}
