package parser

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

type CarunaLoginForm struct {
	ActionURL  *url.URL
	FormValues *url.Values
}

// Used to look for html forms by attributes
type FormQuery struct {
	Id   string
	Name string
}

func FindMetaRefresh(r *bytes.Reader) (string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", err
	}

	url, _ := findFirst(doc, func(n *html.Node) (interface{}, bool) {
		if n.Type == html.ElementNode && n.Data == "meta" {
			var metaType, content string
			for _, attr := range n.Attr {
				switch attr.Key {
				case "http-equiv":
					metaType = attr.Val
				case "content":
					content = attr.Val
				}
			}
			if metaType == "refresh" {
				if i := strings.Index(strings.ToLower(content), "url="); i != -1 {
					return strings.TrimSpace(content[i+4 : len(content)]), true
				}
			}
		}
		return "", false
	})

	if url == nil {
		return "", nil
	}

	return url.(string), nil
}

// Find all the necessary fields for posting login form including csrf token etc
func FindLoginForm(r *bytes.Reader, q *FormQuery) (*CarunaLoginForm, error) {
	ret := &CarunaLoginForm{FormValues: &url.Values{}}

	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}

	// First try to find the form
	form, found := findFirst(doc, func(n *html.Node) (interface{}, bool) {
		if n.Type == html.ElementNode && n.Data == "form" {
			if q != nil {
				for _, attr := range n.Attr {
					// Check for queried attributes

					// Form id
					if q.Id != "" && attr.Key == "id" && attr.Val == q.Id {
						return n, true
					}
					// Form name
					if q.Name != "" && attr.Key == "name" && attr.Val == q.Name {
						return n, true
					}

				}
				// No match, return false
				return nil, false
			}
			// No query, return the first found form
			return n, true
		}
		return nil, false
	})

	if !found {
		return nil, fmt.Errorf("Cannot find Login Form")
	}

	// Form action
	for _, attr := range form.(*html.Node).Attr {
		if attr.Key == "action" {
			ret.ActionURL, err = url.Parse(attr.Val)
			if err != nil {
				return nil, fmt.Errorf("Cannot parse Login form action: %v", err)
			}
		}
	}

	// Find all input fields, store them in return object
	findAll(form.(*html.Node), func(n *html.Node) (interface{}, bool) {
		if n.Type == html.ElementNode && n.Data == "input" {
			var k, v string
			for _, attr := range n.Attr {
				switch attr.Key {
				case "name":
					k = attr.Val
				case "value":
					v = attr.Val
				}
			}
			if k != "" {
				ret.FormValues.Add(k, v)
			}
		}
		// Allways return false as we don't care this time what findAll returns
		return nil, false
	})

	return ret, nil
}

// HTML Parsing helpers //

func findFirst(n *html.Node, fn func(*html.Node) (interface{}, bool)) (interface{}, bool) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if match, found := fn(c); found == true {
			return match, found
		}
		if match, found := findFirst(c, fn); found == true {
			return match, found
		}
	}
	return nil, false
}
func findAll(n *html.Node, fn func(*html.Node) (interface{}, bool)) ([]interface{}, bool) {
	var ret []interface{}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if match, found := fn(c); found == true {
			ret = append(ret, match)
		}
		if match, found := findAll(c, fn); found == true {
			ret = append(ret, match...)
		}
	}
	if len(ret) > 0 {
		return ret, true
	}
	return ret, false
}
