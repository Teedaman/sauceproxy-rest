package main;

import (
	"fmt"
	"testing"
	"strings"
	"net/http"
	"net/http/httptest"
)

const versionJson = `{
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

func TestGetLastVersionBadJSON(t *testing.T) {
	var server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Not a JSON document...")
	}))
	defer server.Close()

	_, _, err := GetLastVersion(server.URL, &http.Transport{})

	if err == nil {
		t.Error("GetLastVersion == nil")
	}

	if !strings.HasPrefix(err.Error(), "Couldn't decode JSON document: ") {
		t.Errorf("Invalid error: %s", err.Error())
	}
}

func TestGetLastVersion404(t *testing.T) {
	var server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nothing to see here", 404)
	}))
	defer server.Close()

	_, _, err := GetLastVersion(server.URL, &http.Transport{})

	if err == nil {
		t.Error("GetLastVersion == nil")
	}

	if !strings.HasPrefix(err.Error(), "Couldn't find ") {
		t.Errorf("Invalid error: %s", err.Error())
	}
}

func TestGetLastVersionNoServer(t *testing.T) {
	var server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))
	// We close the server right-away so it doesn't response to requests, but we
	// still keep it around so our client has a 'bad' URL to connect to.
	server.Close()

	_, _, err := GetLastVersion(server.URL, &http.Transport{})

	if err == nil {
		t.Error("GetLastVersion == nil")
	}

	if !strings.HasPrefix(err.Error(), "Couldn't connect to ") {
		t.Errorf("Invalid error: %s", err.Error())
	}
}
