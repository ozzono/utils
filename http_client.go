package utils

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/pkg/errors"
)

type Client struct {
	method        string
	url           string
	timeout       time.Duration
	retryAttempts int
	retryDelay    time.Duration
	retryRuleF    func(request *Client, response *Response, err error) bool
	param         map[string]string
	query         map[string][]string
	header        map[string][]string
	form          map[string][]string
	body          []byte
	records       interface{}
}

type Response struct {
	StatusCode int
	Header     map[string][]string
	Body       string
}

func NewRest(method string, url string) *Client {
	rest := &Client{
		method:        method,
		url:           url,
		timeout:       2 * time.Second,
		retryAttempts: 0,
		param:         make(map[string]string),
		query:         make(map[string][]string),
		header:        make(map[string][]string),
		form:          make(map[string][]string),
	}
	return rest
}

func add(current []string, values ...interface{}) []string {
	if current == nil {
		current = make([]string, 0)
	}

	for _, value := range values {
		current = append(current, fmt.Sprintf("%v", value))
	}
	return current
}

func (c *Client) Timeout(timeout time.Duration) *Client {
	c.timeout = timeout
	return c
}

func (c *Client) Retry(attempts int, delay time.Duration, ruleF func(request *Client, response *Response, err error) bool) *Client {
	c.retryAttempts = attempts
	c.retryDelay = delay
	c.retryRuleF = ruleF
	return c
}

func (c *Client) Param(param map[string]string) *Client {
	c.param = param
	return c
}

func (c *Client) AddParam(name string, value interface{}) *Client {
	c.param[name] = fmt.Sprintf("%v", value)
	return c
}

func (c *Client) Query(query map[string][]string) *Client {
	c.query = query
	return c
}

func (c *Client) AddQuery(name string, value ...interface{}) *Client {
	c.query[name] = add(c.query[name], value...)
	return c
}

func (c *Client) Header(header map[string][]string) *Client {
	c.header = header
	return c
}

func (c *Client) AddHeader(name string, value ...interface{}) *Client {
	c.header[name] = add(c.header[name], value...)
	return c
}

func (c *Client) Form(form map[string][]string) *Client {
	c.form = form
	return c
}

func (c *Client) AddForm(name string, value ...interface{}) *Client {
	c.form[name] = add(c.form[name], value...)
	return c
}

func (c *Client) Body(body []byte) *Client {
	c.body = body
	return c
}

func (c *Client) Records(records interface{}) *Client {
	c.records = records
	return c
}

func (c *Client) Send() (*Response, error) {
	return c.send(c.retryAttempts)
}

func (c *Client) send(attempts int) (*Response, error) {
	urlParsed, err := url.Parse(c.url)
	if err != nil {
		return nil, errors.Wrap(err, "url.Parse")
	}

	query := url.Values{}

	for name, values := range c.query {
		for _, value := range values {
			query.Add(name, value)
		}
	}

	urlParsed.RawQuery = query.Encode()

	req, err := http.NewRequest(c.method, urlParsed.String(), bytes.NewReader(c.body))
	if err != nil {
		return nil, errors.Wrap(err, "http.NewRequest")
	}

	for name, values := range c.header {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	for name, values := range c.form {
		for _, value := range values {
			req.Form.Add(name, value)
		}
	}

	transport := http.Transport{
		Dial: func(network, addr string) (net.Conn, error) {
			return net.Dial(network, addr)
		},
	}

	httpClient := http.Client{
		Transport: &transport,
		Timeout:   c.timeout,
	}

	var responseErr error
	var response *Response

	var body []byte
	var res *http.Response
	res, responseErr = httpClient.Do(req)

	if responseErr == nil {
		defer res.Body.Close()

		body, responseErr = ioutil.ReadAll(res.Body)

		if responseErr == nil {
			response = &Response{
				StatusCode: res.StatusCode,
				Header:     res.Header,
				Body:       string(body),
			}
		}
	}

	if attempts > 0 {
		if retry := c.retryRuleF(c, response, responseErr); retry {
			time.Sleep(c.retryDelay)
			return c.send(attempts - 1)
		}
	}

	return response, responseErr
}
