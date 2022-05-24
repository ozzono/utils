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

type Rest struct {
	method        string
	url           string
	timeout       time.Duration
	retryAttempts int
	retryDelay    time.Duration
	retryRuleF    func(request *Rest, response *Response, err error) bool
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

func NewRest(method string, url string) *Rest {
	rest := &Rest{
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

func (r *Rest) Timeout(timeout time.Duration) *Rest {
	r.timeout = timeout
	return r
}

func (r *Rest) Retry(attempts int, delay time.Duration, ruleF func(request *Rest, response *Response, err error) bool) *Rest {
	r.retryAttempts = attempts
	r.retryDelay = delay
	r.retryRuleF = ruleF
	return r
}

func (r *Rest) Param(param map[string]string) *Rest {
	r.param = param
	return r
}

func (r *Rest) AddParam(name string, value interface{}) *Rest {
	r.param[name] = fmt.Sprintf("%v", value)
	return r
}

func (r *Rest) Query(query map[string][]string) *Rest {
	r.query = query
	return r
}

func (r *Rest) AddQuery(name string, value ...interface{}) *Rest {
	r.query[name] = add(r.query[name], value...)
	return r
}

func (r *Rest) Header(header map[string][]string) *Rest {
	r.header = header
	return r
}

func (r *Rest) AddHeader(name string, value ...interface{}) *Rest {
	r.header[name] = add(r.header[name], value...)
	return r
}

func (r *Rest) Form(form map[string][]string) *Rest {
	r.form = form
	return r
}

func (r *Rest) AddForm(name string, value ...interface{}) *Rest {
	r.form[name] = add(r.form[name], value...)
	return r
}

func (r *Rest) Body(body []byte) *Rest {
	r.body = body
	return r
}

func (r *Rest) Records(records interface{}) *Rest {
	r.records = records
	return r
}

func (r *Rest) Send() (*Response, error) {
	return r.send(r.retryAttempts)
}

func (r *Rest) send(attempts int) (*Response, error) {
	urlParsed, err := url.Parse(r.url)
	if err != nil {
		return nil, errors.Wrap(err, "url.Parse")
	}

	query := url.Values{}

	for name, values := range r.query {
		for _, value := range values {
			query.Add(name, value)
		}
	}

	urlParsed.RawQuery = query.Encode()

	req, err := http.NewRequest(r.method, urlParsed.String(), bytes.NewReader(r.body))
	if err != nil {
		return nil, errors.Wrap(err, "http.NewRequest")
	}

	for name, values := range r.header {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	for name, values := range r.form {
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
		Timeout:   r.timeout,
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
		if retry := r.retryRuleF(r, response, responseErr); retry {
			time.Sleep(r.retryDelay)
			return r.send(attempts - 1)
		}
	}

	return response, responseErr
}
