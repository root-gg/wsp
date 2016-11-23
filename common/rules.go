package common

import (
	"net/http"
	"regexp"
)

// Rule matchs HTTP requests to allow / deny access
type Rule struct {
	Method string
	URL    string

	methodRegex *regexp.Regexp
	urlRegex    *regexp.Regexp
}

// NewRule create a new Rule
func NewRule(method string, url string) (rule *Rule, err error) {
	rule = new(Rule)
	rule.Method = method
	rule.URL = url
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
	return true
}
