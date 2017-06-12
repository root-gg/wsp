package common

import (
	"fmt"
	"net/http"
	"regexp"
)

// Rule match HTTP requests to allow / deny access
type Rule struct {
	Method  string
	URL     string
	Headers map[string]string

	methodRegex  *regexp.Regexp
	urlRegex     *regexp.Regexp
	headersRegex map[string]*regexp.Regexp
}

// NewRule creates a new Rule
func NewRule(method string, url string, headers map[string]string) (rule *Rule, err error) {
	rule = new(Rule)
	rule.Method = method
	rule.URL = url
	if headers != nil {
		rule.Headers = headers
	} else {
		rule.Headers = make(map[string]string)
	}
	err = rule.Compile()
	return
}

// Compile the regular expressions
func (rule *Rule) Compile() (err error) {
	if rule.Method != "" {
		rule.methodRegex, err = regexp.Compile(rule.Method)
		if err != nil {
			return
		}
	}
	if rule.URL != "" {
		rule.urlRegex, err = regexp.Compile(rule.URL)
		if err != nil {
			return
		}
	}
	rule.headersRegex = make(map[string]*regexp.Regexp)
	for header, regexStr := range rule.Headers {
		var regex *regexp.Regexp
		regex, err = regexp.Compile(regexStr)
		if err != nil {
			return
		}
		rule.headersRegex[header] = regex
	}
	return
}

// Match returns true if the http.Request matches the Rule
func (rule *Rule) Match(req *http.Request) bool {
	if rule.methodRegex != nil && !rule.methodRegex.MatchString(req.Method) {
		return false
	}
	if rule.urlRegex != nil && !rule.urlRegex.MatchString(req.URL.String()) {
		return false
	}

	for headerName, regex := range rule.headersRegex {
		if !regex.MatchString(req.Header.Get(headerName)) {
			return false
		}
	}

	return true
}

func (rule *Rule) String() string {
	return fmt.Sprintf("%s %s %v", rule.Method, rule.URL, rule.Headers)
}
