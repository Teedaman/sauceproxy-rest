package rest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"time"
)

//
// Decode `reader` into the object `v`, and close `reader` after.
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
// SauceProxy control client: allows you to create, query, and shutdown tunnels.
//
type Client struct {
	BaseURL  string
	Username string
	Password string

	// Methods to override default functionality
	DecodeJSON func(reader io.ReadCloser, v interface{}) error
	EncodeJSON func(writer io.Writer, v interface{}) error
	// Execute the request, http.DefaultClient.Do by default
	ExecuteRequest func(*http.Request) (*http.Response, error)
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

	err = c.executeRequest("GET", fullUrl, nil, &jsonStruct)
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

func (c *Client) ReportCrash(tunnel, info, logs string) error {
	var doc = struct {
		Tunnel string `json:"Tunnel"`
		Info   string `json:"Info"`
		Logs   string `json:"Logs"`
	}{Tunnel: tunnel, Info: info, Logs: logs}

	var url = fmt.Sprintf("%s/%s/errors", c.BaseURL, c.Username)

	return c.executeRequest("POST", url, doc, nil)
}

func (c *Client) decode(reader io.ReadCloser, v interface{}) error {
	if reader == nil && v != nil {
		return fmt.Errorf("can't decode JSON from a null reader")
	}
	if c.DecodeJSON != nil {
		return c.DecodeJSON(reader, v)
	} else {
		return decodeJSON(reader, v)
	}
}

func (c *Client) encode(writer io.Writer, v interface{}) error {
	if writer == nil && v != nil {
		return fmt.Errorf("can't encode JSON to a null writer")
	}
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
	method, url string,
	request, response interface{},
) error {
	var reader io.Reader
	// Encode request JSON if needed
	if request != nil {
		var buf bytes.Buffer
		if err := c.encode(&buf, request); err != nil {
			return err
		}
		reader = &buf
	}

	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.Username, c.Password)

	resp, err := func() (*http.Response, error) {
		if c.ExecuteRequest == nil {
			return http.DefaultClient.Do(req)
		} else {
			return c.ExecuteRequest(req)
		}
	}()

	if err != nil {
		return fmt.Errorf("couldn't connect to %s: %s", req.URL, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// there could be an error here in json format
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		return fmt.Errorf(
			"error querying from %s, error was: %s. HTTP status: %s",
			req.URL,
			buf.String(),
			resp.Status)
	}

	// Decode response if needed
	if response != nil {
		err = c.decode(resp.Body, response)
		return err
	}

	return nil
}

type tunnelState struct {
	Id               string   `json:"id"`
	TunnelIdentifier string   `json:"tunnel_identifier"`
	DomainNames      []string `json:"domain_names"`
}

//
// Return the list of tunnel states
//
func (c *Client) listTunnels() (states []tunnelState, err error) {
	var url = fmt.Sprintf("%s/%s/tunnels?full=1", c.BaseURL, c.Username)

	err = c.executeRequest("GET", url, nil, &states)

	return
}

func (c *Client) List() (ids []string, err error) {
	states, err := c.listTunnels()
	if err != nil {
		return
	}

	for _, state := range states {
		ids = append(ids, state.Id)
	}

	return
}

func checkOverlappingDomains(localDomains []string, remoteDomains []string) bool {
	for _, localDomain := range localDomains {
		for _, remoteDomain := range remoteDomains {
			if localDomain == remoteDomain {
				return true
			}
		}
	}
	return false
}

//
// Find tunnels: named tunnel with `name`, or tunnel matching one or more of
// `domains` if name is empty.
//
func (c *Client) Find(name string, domains []string) (
	matches []string, err error,
) {
	list, err := c.listTunnels()
	if err != nil {
		return
	}

	for _, state := range list {
		// If we're an unamed tunnel, check the overlapping domain names
		if name == "" {
			if checkOverlappingDomains(domains, state.DomainNames) {
				matches = append(matches, state.Id)
			}
		// If we're a named tunnel, only check the tunnels' names
		} else if state.TunnelIdentifier == name {
			matches = append(matches, state.Id)
		}
	}

	return
}

//
// Shutdown tunnel `id`
//
func (c *Client) Shutdown(id string) (int, error) {
	return c.shutdown("%s/%s/tunnels/%s", id)
}

func (c *Client) shutdown(urlFmt, id string) (int, error) {
	var url = fmt.Sprintf(urlFmt, c.BaseURL, c.Username, id)

	var response struct {
		JobsRunning int `json:"jobs_running"`
	}
	err := c.executeRequest("DELETE", url, nil, &response)
	jobsRunning := response.JobsRunning

	return jobsRunning, err
}

type Metadata struct {
	Release     string `json:"release"`
	GitVersion  string `json:"git_version"`
	Build       string `json:"build"`
	Platform    string `json:"platform"`
	Hostname    string `json:"hostname"`
	NoFileLimit uint64 `json:"nofile_limit"`
	Command     string `json:"command"`
}

type jsonRequest struct {
	TunnelIdentifier *string   `json:"tunnel_identifier"`
	DomainNames      []string  `json:"domain_names"`
	Metadata         Metadata  `json:"metadata"`
	SSHPort          int       `json:"ssh_port"`
	NoProxyCaching   bool      `json:"no_proxy_caching"`
	UseKGP           bool      `json:"use_kgp"`
	FastFailRegexps  *[]string `json:"fast_fail_regexps"`
	DirectDomains    *[]string `json:"direct_domains"`
	SharedTunnel     bool      `json:"shared_tunnel"`
	SquidConfig      *string   `json:"squid_config"`
	VMVersion        *string   `json:"vm_version"`
	NoSSLBumpDomains *[]string `json:"no_ssl_bump_domains"`
	ExtraInfo        *string   `json:"extra_info"`
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
	Metadata Metadata

	// Extra info. This is a string (which contains a JSON dict) to enable
	// optional features and flags.
	ExtraInfo string
}

// Create a new tunnel and wait for it to come up
//
// This will start a goroutine to keep track of the tunnel's status using the
// ClientStatus & ServerStatus channels
func (c *Client) Create(request *Request) (tunnel Tunnel, err error) {
	tunnel, err = c.CreateWithTimeout(request, time.Minute)

	if err == nil {
		go tunnel.serverStatusLoop(5 * time.Second)
		go tunnel.heartbeatLoop(30 * time.Second)
	}
	return
}

//
// Create a new tunnel and wait for it to come up within `wait`.
//
func (c *Client) CreateWithTimeout(
	request *Request,
	timeout time.Duration,
) (
	tunnel Tunnel, err error,
) {
	var r = request

	var doc = jsonRequest{
		TunnelIdentifier: &r.TunnelIdentifier,
		DomainNames:      r.DomainNames,
		Metadata:         r.Metadata,
		SSHPort:          r.KGPPort,
		NoProxyCaching:   r.NoProxyCaching,
		UseKGP:           true,
		FastFailRegexps:  &r.FastFailRegexps,
		DirectDomains:    &r.DirectDomains,
		SharedTunnel:     r.SharedTunnel,
		SquidConfig:      nil,
		VMVersion:        &r.VMVersion,
		NoSSLBumpDomains: &r.NoSSLBumpDomains,
		ExtraInfo:        &r.ExtraInfo,
	}
	var response struct {
		Id   string `json:"id"`
		Host string `json:"host"`
	}
	var url = fmt.Sprintf("%s/%s/tunnels", c.BaseURL, c.Username)

	err = c.executeRequest("POST", url, doc, &response)
	if err != nil {
		return
	}

	tunnel.Client = c
	tunnel.Id = response.Id
	tunnel.Host, err = tunnel.wait(timeout)
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
	Host   string
	// A channel used to communicate the state of the tunnel back to the main
	// goroutine.
	ServerStatus chan string
	ClientStatus chan ClientStatus
}

func (t *Tunnel) heartbeatLoop(interval time.Duration) {
	var heartbeatTicker = time.NewTicker(interval)
	// Initialize the client status before we start the status loop
	var connected = false
	var lastChange = time.Now()

	for {
		select {
		case clientStatus := <-t.ClientStatus:
			connected = clientStatus.Connected
			lastChange = time.Unix(clientStatus.LastStatusChange, 0)
			var err = t.Client.Ping(t.Id, connected, time.Since(lastChange))
			if err != nil {
				// FIXME old sauceconnect ignores error
			}
		case <-heartbeatTicker.C:
			var err = t.Client.Ping(t.Id, connected, time.Since(lastChange))
			if err != nil {
				// FIXME old sauceconnect ignores error
			}
		}
	}
}

//
// Goroutine that checks if the tunnel is still up and running
//
func (t *Tunnel) serverStatusLoop(interval time.Duration) {
	for range time.NewTicker(interval).C {
		var status, err = t.Status()
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
	}
}

// FIXME the old sauce connect makes an HTTP query and then sleep for 1
// second up to 60 times. This means the old Sauce Connect would wait up to: 60
// seconds + 60 * time the HTTP roundtrip.
//
// Wait for the tunnel to run
func (t *Tunnel) wait(timeout time.Duration) (
	host string,
	err error,
) {
	var end = time.Now().Add(timeout)

	for {
		status, err := t.Client.status(t.Id)
		if err != nil {
			return "", err
		}

		if status.Status == "running" {
			return status.Host, nil
		}

		if time.Now().After(end) {
			break
		} else {
			time.Sleep(time.Second)
		}
	}

	return "", fmt.Errorf(
		"Tunnel %s didn't come up after %s",
		t.Id, timeout.String())
}

func (t *Tunnel) Shutdown() (int, error) {
	return t.Client.shutdown("%s/%s/tunnels/%s?wait_for_jobs=0", t.Id)
}

func (t *Tunnel) ShutdownWaitForJobs() (int, error) {
	return t.Client.shutdown("%s/%s/tunnels/%s?wait_for_jobs=1", t.Id)
}

type serverStatus struct {
	Status       string `json:"status"`
	UserShutdown *bool  `json:"user_shutdown"`
	Host         string `json:"host"`
}

func (c *Client) status(id string) (status serverStatus, err error) {
	var url = fmt.Sprintf("%s/%s/tunnels/%s", c.BaseURL, c.Username, id)

	err = c.executeRequest("GET", url, nil, &status)
	return
}

//
// status can have the values:
// - "running" the tunnel is up and running
// - "halting" the tunnel is shutting down
// - "terminated" the tunnel was shutdown
// - "user shutdown" the tunnel was shutdown by the user from the web interface
//
func (c *Client) Status(id string) (
	status string, err error,
) {
	s, err := c.status(id)
	if err != nil {
		return
	}

	if s.UserShutdown != nil && *s.UserShutdown {
		status = "user shutdown"
	} else {
		status = s.Status
	}

	return
}

func (c *Client) KgpHost(id string) (string, error) {
	var s, err = c.status(id)
	if err != nil {
		return "", err
	}

	return s.Host, nil
}

func (t *Tunnel) Status() (
	status string, err error,
) {
	return t.Client.Status(t.Id)
}

type heartBeatRequest struct {
	KGPConnected         bool  `json:"kgp_is_connected"`
	StatusChangeDuration int64 `json:"kgp_seconds_since_last_status_change"`
}

//
// Send a heartbeat for tunnel `id`
//
func (c *Client) Ping(
	id string,
	connected bool,
	duration time.Duration,
) error {
	var url = fmt.Sprintf("%s/%s/tunnels/%s/connected", c.BaseURL, c.Username, id)

	var h = heartBeatRequest{
		KGPConnected:         connected,
		StatusChangeDuration: int64(duration.Seconds()),
	}

	// The REST call return a JSON document like this:
	//
	//        {"result": true, "id", "<tunnel id>"}
	//
	// We don't decode it since it doesn't give us any useful information to
	// return. It looks like result is always true looking at the REST backend
	// code.
	return c.executeRequest("POST", url, &h, nil)
}
