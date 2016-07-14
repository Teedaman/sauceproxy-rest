package rest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
func (c *Client) GetLastVersion() (
	build int, downloadUrl string, err error,
) {
	// We use only the hostname part of base url
	u, err := url.Parse(c.BaseURL)
	u.Path = ""
	var fullUrl = fmt.Sprintf("%s/versions.json", u)

	body, err := c.executeRequest("GET", fullUrl, nil)
	if err != nil {
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

	if err = decodeJSON(body, &jsonStruct); err != nil {
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
	default:
		build = 0
		downloadUrl = ""
		err = fmt.Errorf("Unknown platform: %v", runtime.GOOS)
		return
	}

	build = x.Build
	downloadUrl = x.DownloadUrl

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
func (c *Client) executeRequest(
	method, url string, body interface{},
) (io.ReadCloser, error) {
	var reader io.Reader
	if body != nil {
		var buf bytes.Buffer
		if err := encodeJSON(&buf, body); err != nil {
			return nil, err
		}
		reader = &buf
	}

	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.Username, c.Password)

	var client = c.Client
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("couldn't connect to %s: %s", req.URL, err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf(
			"error querying from %s. HTTP status: %s",
			req.URL,
			resp.Status)
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

	body, err := c.executeRequest("GET", url, nil)
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
// Find tunnels: named tunnel with `name`, or tunnel matching one or more of
// `domains`.
//
func (c *Client) Find(name string, domains []string) (
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

	_, err := c.executeRequest("DELETE", url, nil)
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

	DirectDomains    []string
	KGPPort          int
	NoProxyCaching   bool
	FastFailRegexps  []string
	SharedTunnel     bool
	VMVersion        string
	NoSSLBumpDomains []string

	// Metadata
	Command string
}

// Create a new tunnel and wait for it to come up
//
// This will start a goroutine to keep track of the tunnel's status using the
// ClientStatus & ServerStatus channels
func (c *Client) Create(request *Request) (tunnel Tunnel, err error) {
	tunnel, err = c.createWithTimeout(request, time.Minute)
	if err == nil {
		go tunnel.loop(
			5*time.Second,
			30*time.Second,
		)
	}
	return
}

const DefaultDomain = "sauce-connect.proxy"

//
// Create a new tunnel and wait for it to come up within `wait`.
//
func (c *Client) createWithTimeout(
	request *Request,
	timeout time.Duration,
) (
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
	var domainNames []string
	if r.DomainNames == nil {
		if r.TunnelIdentifier == "" {
			domainNames = []string{DefaultDomain}
		}
	} else {
		domainNames = r.DomainNames
	}

	var doc = jsonRequest{
		TunnelIdentifier: &r.TunnelIdentifier,
		DomainNames:      domainNames,
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
	var url = fmt.Sprintf("%s/%s/tunnels", c.BaseURL, c.Username)

	body, err := c.executeRequest("POST", url, doc)
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
	err = tunnel.wait(timeout)
	// Only create channels if the tunnel succesfully come up
	if err == nil {
		tunnel.ServerStatus = make(chan string)
		tunnel.ClientStatus = make(chan ClientStatus)
	}
	return
}

type ClientStatus struct {
	Connected        bool
	LastStatusChange int64
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
	ServerStatus chan string
	ClientStatus chan ClientStatus
}

//
// Goroutine that checks if the tunnel is still up and running, and sends a
// heart beat to indicate the tunnel client is still up.
//
func (t *Tunnel) loop(
	serverStatusInterval time.Duration,
	heartbeatInterval time.Duration,
) {
	var termTick = time.Tick(serverStatusInterval)
	var heartbeatTick = time.Tick(heartbeatInterval)
	// Initialize the client status before we start the status loop
	var clientStatus ClientStatus = <-t.ClientStatus
	var connected = clientStatus.Connected
	var lastChange = time.Unix(clientStatus.LastStatusChange, 0)

	for {
		select {
		case clientStatus = <-t.ClientStatus:
			connected = clientStatus.Connected
			lastChange = time.Unix(clientStatus.LastStatusChange, 0)
		case <-termTick:
			var status, err = t.status()
			if err != nil {
				// FIXME old sauceconnect ignores error
			} else if status != "running" {
				//
				// The tunnel is down, send its status back to the main loop.
				//
				t.ServerStatus <- status
				close(t.ServerStatus)
				return // We're done exit the loop
			}
		case <-heartbeatTick:
			var err = t.sendHeartBeat(
				connected,
				time.Since(lastChange),
			)
			if err != nil {
				// FIXME old sauceconnect ignores error
			}
		}
	}
}

// FIXME the old sauce connect makes an HTTP query and then sleep for 1
// second up to 60 times. This means the old Sauce Connect would wait up to: 60
// seconds + 60 * time the HTTP roundtrip.
//
// Wait for the tunnel to run
func (t *Tunnel) wait(timeout time.Duration) error {
	var now = time.Now()
	var end = now.Add(timeout)

	for !now.After(end) {
		status, err := t.status()
		if err != nil {
			return err
		}

		if status == "running" {
			return nil
		} else {
			time.Sleep(time.Second)
		}
		now = time.Now()
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
// - "halting" the tunnel is shutting down
// - "terminated" the tunnel was shutdown
// - "user shutdown" the tunnel was shutdown by the user from the web interface
//
// If the query failed status will return an error.
//
func (t *Tunnel) status() (
	status string, err error,
) {
	var c = t.Client
	var url = fmt.Sprintf("%s/%s/tunnels/%s", c.BaseURL, c.Username, t.Id)

	body, err := c.executeRequest("GET", url, nil)
	if err != nil {
		return
	}

	var s struct {
		Status       string `json:"status"`
		UserShutdown *bool  `json:"user_shutdown"`
	}

	if err = c.decodeJSON(body, &s); err != nil {
		return
	}

	if s.UserShutdown != nil && *s.UserShutdown {
		status = "user shutdown"
	} else {
		status = s.Status
	}

	return
}

func (t *Tunnel) sendHeartBeat(
	connected bool,
	duration time.Duration,
) error {
	var c = t.Client
	var url = fmt.Sprintf("%s/%s/tunnels/%s/connected", c.BaseURL, c.Username, t.Id)

	var h = struct {
		KGPConnected         bool  `json:"kgp_is_connected"`
		StatusChangeDuration int64 `json:"kgp_seconds_since_last_status_change"`
	}{
		KGPConnected:         connected,
		StatusChangeDuration: int64(duration.Seconds()),
	}

	// The REST call return a JSON document like this:
	//
	//	  {"result": true, "id", "<tunnel id>"}
	//
	// We don't decode it since it doesn't give us any useful information to
	// return.
	//
	// FIXME it looks like result is always true looking at the Resto code
	_, err := c.executeRequest("POST", url, &h)
	if err != nil {
		return err
	}

	return nil
}
