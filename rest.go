package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	_ "io/ioutil"
	"net/http"
	"os"
	"runtime"
	"time"

	"golang.org/x/sys/unix"
)

//
// Decode `reader` into the object `v`, and close `reader` after.
//
//
func decodeJSON(reader io.ReadCloser, v interface{}) error {
	var err = json.NewDecoder(reader).Decode(v)
	reader.Close()
	if err != nil {
		return fmt.Errorf("couldn't decode JSON document: %s", err)
	}

	return nil
}

func encodeJSON(w io.Writer, v interface{}) error {
	var err = json.NewEncoder(w).Encode(v)
	if err != nil {
		return fmt.Errorf("couldn't encode JSON document: %s", err)
	}

	return nil
}

//
// Query `baseURL/versions.json` for a new version of Sauce Connect
//
// Return the newest build number for the platform as determined by
// runtime.GOOS, and the URL to download the latest verion.
//
func GetLastVersion(baseUrl string, client *http.Client) (
	build int, url string, err error,
) {
	var fullUrl = fmt.Sprintf("%s/versions.json", baseUrl)

	resp, err := client.Get(fullUrl)
	if err != nil {
		err = fmt.Errorf("couldn't connect to %s: %s", fullUrl, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("couldn't find %s: %s", fullUrl, resp.Status)
		return
	}

	// Structures we use to decode the json document
	type jsonBuild struct {
		Build       int
		DownloadUrl string `json:"download_url"`
		Sha1        string
	}

	var jsonStruct = struct {
		SauceConnect struct {
			Linux   jsonBuild `json:"linux"`
			Linux32 jsonBuild `json:"linux32"`
			Osx     jsonBuild `json:"osx"`
			Win32   jsonBuild `json:"win32"`
		} `json:"Sauce Connect"`
	}{}

	err = decodeJSON(resp.Body, &jsonStruct)
	if err != nil {
		return
	}

	var p = jsonStruct.SauceConnect
	var x jsonBuild

	switch runtime.GOOS {
	case "windows":
		x = p.Win32
	case "linux":
		switch runtime.GOARCH {
		case "386":
			x = p.Linux32
		case "amd64":
			x = p.Linux
		}
	case "darwin":
		x = p.Osx
	}

	build = x.Build
	url = x.DownloadUrl

	return
}

//
// SauceProxy control client: allows you to create, query, and shutdown tunnels.
//
type Client struct {
	BaseURL  string
	Username string
	Password string

	Client http.Client

	// Methods to override the default decoding function
	DecodeJSON func(reader io.ReadCloser, v interface{}) error
	EncodeJSON func(writer io.Writer, v interface{}) error
}

func (c *Client) decodeJSON(reader io.ReadCloser, v interface{}) error {
	if c.DecodeJSON != nil {
		return c.DecodeJSON(reader, v)
	} else {
		return decodeJSON(reader, v)
	}
}

func (c *Client) encodeJSON(writer io.Writer, v interface{}) error {
	if c.EncodeJSON != nil {
		return c.EncodeJSON(writer, v)
	} else {
		return encodeJSON(writer, v)
	}
}

//
// Execute HTTP request and return an io.ReadCloser to be decoded
//
func (c *Client) executeRequest(req *http.Request) (io.ReadCloser, error) {
	req.SetBasicAuth(c.Username, c.Password)

	var client = c.Client
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("couldn't connect to %s: %s", req.URL, err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("couldn't find %s: %s", req.URL, resp.Status)
	}

	return resp.Body, nil
}

type tunnelState struct {
	Id               string   `json:"id"`
	TunnelIdentifier string   `json:"tunnel_id"`
	DomainNames      []string `json:"domain_names"`
}

//
// Return the list of tunnel states
//
func (c *Client) list() (states []tunnelState, err error) {
	var url = fmt.Sprintf("%s/%s/tunnels?full=1", c.BaseURL, c.Username)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}

	body, err := c.executeRequest(req)
	if err != nil {
		return
	}

	err = c.decodeJSON(body, &states)
	if err != nil {
		return
	}

	return
}

//
// Match tunnels: named tunnel with `name`, or tunnel matching one or more of
// `domains`.
//
func (c *Client) Match(name string, domains []string) (
	matches []string, err error,
) {
	list, err := c.list()
	if err != nil {
		return
	}

	for _, state := range list {
		if state.TunnelIdentifier == name {
			matches = append(matches, state.Id)
			continue
		}

		for _, localDomain := range domains {
			for _, remoteDomain := range state.DomainNames {
				if localDomain == remoteDomain {
					matches = append(matches, state.Id)
					continue
				}
			}
		}
	}

	return
}

//
// Shutdown tunnel `id`
//
func (c *Client) Shutdown(id string) error {
	return c.shutdown("%s/%s/tunnels/%s", id)
}

func (c *Client) shutdown(urlFmt, id string) error {
	var url = fmt.Sprintf(urlFmt, c.BaseURL, c.Username, id)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	_, err = c.executeRequest(req)
	if err != nil {
		return err
	}

	return nil
}

type jsonMetadata struct {
	Release     string `json:"release"`
	GitVersion  string `json:"git_version"`
	Build       string `json:"build"`
	Platform    string `json:"platform"`
	Hostname    string `json:"hostname"`
	NoFileLimit uint64 `json:"no_file_limit"`
	Command     string `json:"command"`
}

type jsonRequest struct {
	TunnelIdentifier *string      `json:"tunnel_identifier"`
	DomainNames      []string     `json:"domain_names"`
	Metadata         jsonMetadata `json:"metadata"`
	SSHPort          int          `json:"ssh_port"`
	NoProxyCaching   bool         `json:"no_proxy_caching"`
	UseKGP           bool         `json:"use_kgp"`
	FastFailRegexps  *[]string    `json:"fast_fail_regexps"`
	DirectDomains    *[]string    `json:"direct_domains"`
	SharedTunnel     bool         `json:"shared_tunnel"`
	SquidConfig      *string      `json:"squid_config"`
	VMVersion        *string      `json:"vm_version"`
	NoSSLBumpDomains *[]string    `json:"no_ssl_bump_domains"`
}

//
// Request for a new tunnel
//
type Request struct {
	TunnelIdentifier string
	DomainNames      []string

	KGPPort          int
	NoProxyCaching   bool
	FastFailRegexps  []string
	DirectDomains    []string
	SharedTunnel     bool
	VMVersion        string
	NoSSLBumpDomains []string

	// Metadata
	Command string
}

func (c *Client) Create(request *Request) (tunnel Tunnel, err error) {
	var timeout = time.Minute
	var wait = time.Minute

	return c.createWithTimeouts(request, timeout, wait)
}

//
// Create a new tunnel and wait for it to come up within `timeout`.
//
// This will start a goroutine to keep track of the tunnel's status.
//
func (c *Client) createWithTimeouts(request *Request, timeout time.Duration, wait time.Duration) (
	tunnel Tunnel, err error,
) {
	hostname, err := os.Hostname()
	if err != nil {
		return
	}
	var rlimit unix.Rlimit
	err = unix.Getrlimit(unix.RLIMIT_NOFILE, &rlimit)
	if err != nil {
		return
	}
	var r = request
	var doc = jsonRequest{
		TunnelIdentifier: &r.TunnelIdentifier,
		DomainNames:      r.DomainNames,
		Metadata: jsonMetadata{
			Release:     "4.3.99",
			GitVersion:  "123467",
			Build:       "1234",
			Platform:    "plan9",
			Hostname:    hostname,
			NoFileLimit: rlimit.Cur,
			Command:     r.Command,
		},
		SSHPort:          r.KGPPort,
		NoProxyCaching:   r.NoProxyCaching,
		UseKGP:           true,
		FastFailRegexps:  &r.FastFailRegexps,
		DirectDomains:    &r.DirectDomains,
		SharedTunnel:     r.SharedTunnel,
		SquidConfig:      nil,
		VMVersion:        &r.VMVersion,
		NoSSLBumpDomains: &r.NoSSLBumpDomains,
	}
	var jsonDoc bytes.Buffer
	if err = encodeJSON(&jsonDoc, doc); err != nil {
		return
	}

	var url = fmt.Sprintf("%s/%s/tunnels", c.BaseURL, c.Username)
	req, err := http.NewRequest("POST", url, &jsonDoc)
	if err != nil {
		return
	}

	body, err := c.executeRequest(req)
	if err != nil {
		return
	}

	var response struct {
		Id string
	}

	err = c.decodeJSON(body, &response)
	if err != nil {
		return
	}

	tunnel.Client = c
	tunnel.Id = response.Id
	err = tunnel.wait(wait)
	return
}

//
// Tunnel control interface. Create it by calling Client.Create(), all methods
// are safe to call across goroutines. Tunnel.Status() is updated every XXX
// seconds by a goroutine that queries the state of the tunnel.
//
// We may want to switch the method Status with a direct access to the active
// channel instead depending of how the main loop is done.
//
type Tunnel struct {
	Client *Client
	Id     string

	// A channel used to communicate the state of the tunnel back to the main
	// goroutine.
	active chan string
}

// FIXME the old sauce connect makes an HTTP query and then sleep for 1
// second up to 60 times. This means the old Sauce Connect would wait up to: 60
// seconds + 60 * time the HTTP roundtrip.
//
// Wait for the tunnel to run
func (t *Tunnel) wait(timeout time.Duration) error {
	var end = time.Now().Add(timeout)

	for time.Now().Before(end) {
		status, err := t.status()
		if err != nil {
			return err
		}

		if status == "running" {
			return nil
		} else {
			time.Sleep(time.Second)
		}
	}

	return fmt.Errorf(
		"Tunnel %s didn't come up after %s",
		t.Id, timeout.String())
}

func (t *Tunnel) Shutdown() error {
	return t.Client.shutdown("%s/%s/tunnels/%s", t.Id)
}

func (t *Tunnel) ShutdownWaitForJobs() error {
	return t.Client.shutdown("%s/%s/tunnels/%s?wait_for_jobs=1", t.Id)
}

//
// status can have the values:
// - "running" the tunnel is up and running
// - "terminated" the tunnel isn't running (it's assumed it was terminated, but it could be any state that's != "running")
// - "user shutdown" the tunnel was shutdown by the user from the web interface
//
// If the query failed status will return an error.
//
func (t *Tunnel) status() (
	status string, err error,
) {
	var c = t.Client
	var url = fmt.Sprintf("%s/%s/tunnels/%s", c.BaseURL, c.Username, t.Id)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}

	body, err := c.executeRequest(req)
	if err != nil {
		return
	}

	var s struct {
		Status       string `json:"status"`
		UserShutdown *bool `json:"user_shutdown"`
	}

	if err = c.decodeJSON(body, &s); err != nil {
		return
	}

	if s.UserShutdown != nil && *s.UserShutdown {
		status = "user shutdown"
	} else if s.Status != "running" {
		status = "terminated"
	} else {
		status = "running"
	}

	return
}

/*
type heartBeat struct {
	KGPConnected         bool `json:"kgp_is_connected"`
	StatusChangeDuration int  `json:"kgp_seconds_since_last_status_change"`
}

func SendHeartBeat(
	id string,
	connected bool,
	duration time.Duration,
	config *RequestConfig,
) error {
	var url = fmt.Sprintf("%s/%s/tunnels/%s/connected", config.BaseURL, config.Username, id)

	var h = heartBeat{
		KGPConnected:         connected,
		StatusChangeDuration: int(duration.Seconds()),
	}
	var jsonDoc bytes.Buffer
	if err := encodeJSON(&jsonDoc, &h); err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, &jsonDoc)
	if err != nil {
		return err
	}

	//
	// The REST call return a JSON document like this:
	//
	//	  {"result": true, "id", "<tunnel id>"}
	//
	// We don't decode it since it doesn't give us any information to return
	//
	_, err = executeRequest(req, config)
	if err != nil {
		return err
	}

	return nil
}

// FIXME the old sauce connect makes an HTTP query and then sleep for 1
// second up to 60 times. This means the old Sauce Connect would wait up to: 60
// seconds + 60 * time the HTTP roundtrip.
//
// This means we may have to bump this timeout value up from 1 minute.
const waitTimeout = time.Minute

func WaitForTunnel(id string, config *RequestConfig) error {
	var timeout = time.Now().Add(waitTimeout)

	for time.Now().Before(timeout) {
		status, err := getTunnelStatus(id, config)
		if err != nil {
			return err
		}

		if status.Status == "running" {
			return nil
		} else {
			time.Sleep(time.Second)
		}
	}

	return fmt.Errorf(
		"Tunnel %s didn't come up after %s",
		id, waitTimeout.String())
}

*/

/*
func (t *Tunnel) Status() error {

}
*/

/*
client := Client("http://...", "username", "password")

overlapping, _ := client.Match("<tunnel id>", []string{"sauce-connect.proxy"})
if args.Remove {
	for _, tid := range overlapping {
		_ = Client.Remove(tid)
	}
} else {
	log.Printf("Overlapping tunnels: %s\n", overlapping)
}

request := Request{
	TunnelIdentifier: "<tunnel id>",
	DomainNames: []string{"foo.bar", "saucelabs.com"},
}
timeout := time.Minute
tunnel, _ := client.Create(request, timeout)

...

err := tunnel.Status() // Status is checked in another goroutine
if err != nil {
	log.Printf("Tunnel got shut down: %s\n", err)
	break
}
...
_ := tunnel.Shutdown()
*/
