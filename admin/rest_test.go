package admin

import (
	"reflect"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

// Helper function to create a fake http server
func makeServer(f func(w http.ResponseWriter)) *httptest.Server {
	return httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			f(w)
		}))
}

func TestGetLastVersion(t *testing.T) {
	var server = makeServer(func(w http.ResponseWriter) {
		fmt.Fprintln(w, versionJson)
	})
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
	var server = makeServer(func(w http.ResponseWriter) {
		fmt.Fprintln(w, "Not a JSON document...")
	})
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
	var server = makeServer(func(w http.ResponseWriter) {
		http.Error(w, "nothing to see here", 404)
	})
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
	var server = makeServer(func(w http.ResponseWriter) {})
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

const tunnelsJSON = `[
  {
    "status": "running",
    "direct_domains": null,
    "vm_version": null,
    "last_connected": 1467691618,
    "shutdown_time": null,
    "ssh_port": 443,
    "launch_time": 1467690963,
    "user_shutdown": null,
    "use_caching_proxy": null,
    "creation_time": 1467690959,
    "domain_names": [
      "sauce-connect.proxy"
    ],
    "shared_tunnel": false,
    "tunnel_identifier": null,
    "host": "maki81134.miso.saucelabs.com",
    "no_proxy_caching": false,
    "owner": "henryprecheur",
    "use_kgp": true,
    "no_ssl_bump_domains": null,
    "id": "fakeid",
    "metadata": {
      "hostname": "debian-desktop",
      "git_version": "39e807b",
      "platform": "Linux 4.6.0-1-amd64 #1 SMP Debian 4.6.2-2 (2016-06-25) x86_64",
      "command": "./sc -u henryprecheur -k ****",
      "build": "2396",
      "release": "4.3.16",
      "nofile_limit": 1024
    }
  }
]`

func TestTunnelStates(t *testing.T) {
	var server = makeServer(func(w http.ResponseWriter) {
		fmt.Fprintln(w, tunnelsJSON)
	})
	defer server.Close()

	var config = RequestConfig{
		BaseURL: server.URL,
		Username: "username",
		Password: "password",
		Transport: &http.Transport{},
	}
	states, err := GetTunnelStates(&config)
	if err != nil {
		t.Errorf("GetTunnelStates returned: %s", err)
	}

	var expected = TunnelStates{{
			Id: "fakeid",
			TunnelIdentified: "", // FIXME is null == "" a good assumption?
			DomainNames: []string{"sauce-connect.proxy"},
		}}
	if !reflect.DeepEqual(states, expected) {
		t.Errorf("GetTunnelStates returned: %+v\n", states)
	}

	var matches = states.Match("otherid", []string{"sauce-connect.proxy"})
	if !reflect.DeepEqual(matches, expected) {
		t.Errorf("states.Match returned: %+v\n", states)
	}

	var emptyMatches = states.Match("otherid", []string{"bad.domain.proxy"})
	if !reflect.DeepEqual(emptyMatches, TunnelStates{}) {
		t.Errorf("states.Match returned: %+v\n", emptyMatches)
	}
}
