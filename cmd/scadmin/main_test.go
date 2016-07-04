package main;

import (
	"fmt"
	"testing"
	"net/http"
	"net/http/httptest"
)

const versionJson = `
{
    "Sauce Connect": {
        "download_url": "https://wiki.saucelabs.com/display/DOCS/Setting+Up+Sauce+Connect", 
        "linux": {
            "build": 42, 
            "download_url": "https://saucelabs.com/downloads/sc-new",
            "sha1": "123456"
        }, 
        "linux32": {
            "build": 42, 
            "download_url": "https://saucelabs.com/downloads/sc-new",
            "sha1": "123456"
        }, 
        "osx": {
            "build": 42, 
            "download_url": "https://saucelabs.com/downloads/sc-new",
            "sha1": "123456"
        }, 
        "version": "4.3.16", 
        "win32": {
            "build": 42, 
            "download_url": "https://saucelabs.com/downloads/sc-new",
            "sha1": "123456"
        }
    }, 
    "Sauce Connect 2": {
        "download_url": "https://docs.saucelabs.com/reference/sauce-connect/", 
        "version": "4.3.13-r999"
    }
}`

func TestGetLastVersion(t *testing.T) {
	var server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, versionJson)
	}))
	defer server.Close()

	build, url, err := GetLastVersion(server.URL, &http.Transport{})

	if err != nil {
		t.Errorf("%v", err)
	}
	if build != 42 {
		t.Errorf("Bad build number: %d", build)
	}
	if url != "https://saucelabs.com/downloads/sc-new" {
		t.Errorf("Bad URL: %s", url)
	}
}
