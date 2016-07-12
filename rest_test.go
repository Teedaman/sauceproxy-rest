package admin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
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

// Helper type to make declarations shorter
type R func(http.ResponseWriter, *http.Request)

func stringResponse(s string) R {
	return func(r http.ResponseWriter, q *http.Request) {
		io.WriteString(r, s)
	}
}

func errorResponse(code int, s string) R {
	return func(r http.ResponseWriter, _ *http.Request) {
		http.Error(r, s, code)
	}
}

// Return each response one after another, keeps repeating the last response
// once it has reached the end.
func multiResponseServer(responses []R) *httptest.Server {
	var index = 0
	return httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			responses[index](w, r)
			if index < len(responses) {
				index += 1
			}
		}))
}

func TestGetLastVersion(t *testing.T) {
	var server = multiResponseServer([]R{
		// Just return a fake version.json
		stringResponse(versionJson),
	})
	defer server.Close()

	var client = Client{
		BaseURL: server.URL,
	}
	build, url, err := client.GetLastVersion()

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
	var server = multiResponseServer([]R{
		stringResponse("Not a JSON document"),
	})
	defer server.Close()

	var client = Client{
		BaseURL: server.URL,
	}
	_, _, err := client.GetLastVersion()

	if err == nil {
		t.Error("GetLastVersion == nil")
	}

	if !strings.HasPrefix(err.Error(), "couldn't decode JSON document: ") {
		t.Errorf("Invalid error: %s", err.Error())
	}
}

func TestGetLastVersion404(t *testing.T) {
	var server = multiResponseServer([]R{
		errorResponse(404, "Nothing to see here"),
	})
	defer server.Close()

	var client = Client{
		BaseURL: server.URL,
	}
	_, _, err := client.GetLastVersion()

	if err == nil {
		t.Error("GetLastVersion == nil")
	}

	if !strings.HasPrefix(err.Error(), "couldn't find ") {
		t.Errorf("Invalid error: %s", err.Error())
	}
}

func TestGetLastVersionNoServer(t *testing.T) {
	var server = multiResponseServer([]R{})
	// We close the server right-away so it doesn't response to requests, but we
	// still keep it around so our client has a 'bad' URL to connect to.
	server.Close()

	var client = Client{
		BaseURL: server.URL,
	}
	_, _, err := client.GetLastVersion()

	if err == nil {
		t.Error("GetLastVersion == nil")
	}

	if !strings.HasPrefix(err.Error(), "couldn't connect to ") {
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

func TestClientMatch(t *testing.T) {
	var server = multiResponseServer([]R{
		stringResponse(tunnelsJSON),
	})
	defer server.Close()

	var client = Client{
		BaseURL:  server.URL,
		Username: "username",
		Password: "password",
	}

	var matches, err = client.Match("fakeid", []string{"sauce-connect.proxy"})

	if err != nil {
		t.Errorf("client.Match errored %+v\n", err)
	}

	if !reflect.DeepEqual(matches, []string{"fakeid"}) {
		t.Errorf("client.Match returned %+v\n", matches)
	}
}

func TestClientShutdown(t *testing.T) {
	var server = multiResponseServer([]R{
		stringResponse(""),
	})
	defer server.Close()

	var client = Client{
		BaseURL:  server.URL,
		Username: "username",
		Password: "password",
	}

	err := client.Shutdown("fakeid")
	if err != nil {
		t.Errorf("client.Shutdown errored %+v\n", err)
	}
}

func TestClientShutdown404(t *testing.T) {
	var server = multiResponseServer([]R{
		errorResponse(404, "nothing to see here"),
	})
	defer server.Close()

	var client = Client{
		BaseURL:  server.URL,
		Username: "username",
		Password: "password",
	}

	err := client.Shutdown("fakeid")
	if !strings.HasPrefix(err.Error(), "couldn't find ") {
		t.Errorf("Invalid error: %s", err.Error())
	}
}

const createJSON = `{
  "status": "new",
  "direct_domains": null,
  "vm_version": null,
  "last_connected": null,
  "shutdown_time": null,
  "ssh_port": 443,
  "launch_time": null,
  "user_shutdown": null,
  "use_caching_proxy": null,
  "creation_time": 1467839998,
  "domain_names": [
    "sauce-connect.proxy"
  ],
  "shared_tunnel": false,
  "tunnel_identifier": null,
  "host": null,
  "no_proxy_caching": false,
  "owner": "henryprecheur",
  "use_kgp": true,
  "no_ssl_bump_domains": null,
  "id": "49958ce5ec9f49c796542e0c691455a6",
  "metadata": {
    "hostname": "Henry's computer",
    "git_version": "4a804fd",
    "platform": "plan9",
    "command": "./sc",
    "build": "Strong",
    "release": "1.2.3",
    "no_file_limit": 12345
  }
}`

const statusRunningJSON = `{"status": "running", "user_shutdown": null}`

func createTunnel(url string) (Tunnel, error) {
	var client = Client{
		BaseURL:  url,
		Username: "username",
		Password: "password",
	}
	var request = Request{
		DomainNames: []string{"sauce-connect.proxy"},
	}
	return client.createWithTimeout(&request, 0)
}

func TestClientCreate(t *testing.T) {
	var server = multiResponseServer([]R{
		stringResponse(createJSON),
		stringResponse(statusRunningJSON),
	})
	defer server.Close()

	_, err := createTunnel(server.URL)
	if err != nil {
		t.Errorf("client.createWithTimeout errored %+v\n", err)
	}
}

func TestClientCreateHTTPError(t *testing.T) {
	var server = multiResponseServer([]R{
		errorResponse(504, "Not available"),
	})
	defer server.Close()

	_, err := createTunnel(server.URL)
	if err == nil {
		t.Errorf("client.createWithTimeout didn't error")
	}

	if !(strings.HasPrefix(err.Error(), "couldn't find ") &&
		strings.HasSuffix(err.Error(), "504 Gateway Timeout")) {
		t.Errorf("Invalid error: %s", err.Error())
	}
}

func TestClientCreateWaitError(t *testing.T) {
	var server = multiResponseServer([]R{
		stringResponse(createJSON),
		stringResponse(`{"status": "shutdown", "user_shutdown": null}`),
	})
	defer server.Close()

	_, err := createTunnel(server.URL)
	if err == nil {
		t.Errorf("client.createWithTimeout didn't error")
	}

	if !(strings.HasPrefix(err.Error(), "Tunnel ") &&
		strings.HasSuffix(err.Error(), " didn't come up after 0")) {
		t.Errorf("Invalid error: %s", err.Error())
	}
}

func TestTunnelHeartBeat(t *testing.T) {
	var server = multiResponseServer([]R{
		stringResponse(createJSON),
		stringResponse(statusRunningJSON),
		stringResponse(
			`{"result": true, "id": "49958ce5ec9f49c796542e0c691455a6"}`),
	})
	defer server.Close()

	tunnel, err := createTunnel(server.URL)
	if err != nil {
		t.Errorf("client.createWithTimeout errored %+v\n", err)
	}

	err = tunnel.sendHeartBeat(true, time.Hour)
	if err != nil {
		t.Errorf("tunnel.sendHeartBeat errored %+v\n", err)
	}
}

func TestTunnelHeartBeatError(t *testing.T) {
	var server = multiResponseServer([]R{
		stringResponse(createJSON),
		stringResponse(statusRunningJSON),
	})

	tunnel, err := createTunnel(server.URL)
	if err != nil {
		t.Errorf("client.createWithTimeout errored %+v\n", err)
	}

	server.Close()
	err = tunnel.sendHeartBeat(true, time.Hour)
	if err == nil {
		t.Errorf("tunnel.sendHeartBeat didn't error\n")
	}
	if !strings.HasPrefix(err.Error(), "couldn't connect to ") {
		t.Errorf("Invalid error: %s", err.Error())
	}
}

// Run until server shuts down
func TestTunnelLoop(t *testing.T) {
	var server = multiResponseServer([]R{
		stringResponse(createJSON),
		stringResponse(statusRunningJSON),
		stringResponse(statusRunningJSON),
		stringResponse(`{"result": true, "tunnel_id": "fakeid"}`),
		stringResponse(`{"status": "shutdown", "user_shutdown": null}`),
	})

	tunnel, err := createTunnel(server.URL)
	if err != nil {
		t.Errorf("client.createWithTimeout errored %+v\n", err)
	}
	go tunnel.loop(
		2 * time.Millisecond, // Make sure status check happens before heartbeat
		3 * time.Millisecond,
	)
	// Notify the Tunnel object that KGP is "up"
	tunnel.ClientStatus <- ClientStatus{
		Connected: true,
		LastStatusChange: 0,
	}

	var serverStatus = <-tunnel.ServerStatus
	if serverStatus != "shutdown" {
		t.Errorf("Invalid server status %+v\n", serverStatus)
	}
	_, ok := <-tunnel.ServerStatus
	if ok {
		t.Errorf("ServerStatus wasn't closed")
	}
}

func heartbeatChecker(connected bool, changeDuration int64, t *testing.T) (
	func(http.ResponseWriter, *http.Request),
) {
	return func(w http.ResponseWriter, r *http.Request) {
		var h struct {
			KGPConnected         bool `json:"kgp_is_connected"`
			StatusChangeDuration int64  `json:"kgp_seconds_since_last_status_change"`
		}
		if err := decodeJSON(r.Body, &h); err != nil {
			t.Errorf("decodeJSON errored %+v\n", err)
		}
		if (
			h.KGPConnected != connected ||
			h.StatusChangeDuration != changeDuration) {
			t.Errorf(
				"Invalid values: %v, %v != %v %v\n",
				h.KGPConnected,
				h.StatusChangeDuration,
				connected,
				changeDuration,
			)
		}
	}
}

// Run until KGP client shuts down
func TestTunnelLoopClientStop(t *testing.T) {
	var server = multiResponseServer([]R{
		stringResponse(createJSON),
		stringResponse(statusRunningJSON),
		heartbeatChecker(true, 1, t),
		heartbeatChecker(false, 0, t),
		stringResponse(`{"status": "shutdown", "user_shutdown": null}`),
	})

	tunnel, err := createTunnel(server.URL)
	if err != nil {
		t.Errorf("client.createWithTimeout errored %+v\n", err)
	}
	go tunnel.loop(
		// Using the timing of the 2 loops to ensure that we get 2 heartbeats
		// before we check if the server is up.
		5 * time.Millisecond,
		2 * time.Millisecond,
	)
	var now = time.Now()
	var before = now.Add(-1 * time.Second)
	// Notify the Tunnel object that KGP is "up"
	tunnel.ClientStatus <- ClientStatus{
		Connected: true,
		LastStatusChange: before.Unix(),
	}
	time.Sleep(3 * time.Millisecond)  // Make sure the request was handled
	// Notify the tunnel the KGP went "down"
	tunnel.ClientStatus <- ClientStatus{
		Connected: false,
		LastStatusChange: now.Unix(),
	}

	var serverStatus = <-tunnel.ServerStatus
	if serverStatus != "shutdown" {
		t.Errorf("Invalid server status %+v\n", serverStatus)
	}
}
