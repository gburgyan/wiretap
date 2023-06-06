// Copyright 2023 Princess B33f Heavy Industries / Dave Shanley
// SPDX-License-Identifier: MIT

package daemon

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/pb33f/wiretap/shared"
)

type wiretapTransport struct {
	capturedCookieHeaders []string
	originalTransport     http.RoundTripper
}

func newWiretapTransport() *wiretapTransport {
	// Disable ssl cert checks
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	return &wiretapTransport{
		originalTransport: http.DefaultTransport,
	}
}

func (c *wiretapTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := c.originalTransport.RoundTrip(r)
	if resp != nil {
		cookie := resp.Header.Get("Set-Cookie")
		if cookie != "" {
			c.capturedCookieHeaders = append(c.capturedCookieHeaders, cookie)
		}
	}
	return resp, err
}

func (ws *WiretapService) callAPI(req *http.Request) (*http.Response, error) {

	tr := newWiretapTransport()
	client := &http.Client{Transport: tr}

	configStore, _ := ws.controlsStore.Get(shared.ConfigKey)

	// create a new request from the original request, but replace the path
	config := configStore.(*shared.WiretapConfiguration)
	newReq := cloneRequest(req,
		config.RedirectProtocol,
		config.RedirectHost,
		config.RedirectPort)

	// re-write referer
	if newReq.Header.Get("Referer") != "" {
		// retain original referer for logging
		newReq.Header.Set("X-Original-Referer", newReq.Header.Get("Referer"))
		newReq.Header.Set("Referer", reconstructURL(req,
			config.RedirectProtocol,
			config.RedirectHost,
			config.RedirectPort))
	}
	resp, err := client.Do(newReq)

	if err != nil {
		return nil, err
	}

	fmt.Print(tr.capturedCookieHeaders)

	if len(tr.capturedCookieHeaders) > 0 {
		if resp.Header.Get("Set-Cookie") == "" {
			resp.Header.Set("Set-Cookie", tr.capturedCookieHeaders[0])
		}
	}
	return resp, nil
}
