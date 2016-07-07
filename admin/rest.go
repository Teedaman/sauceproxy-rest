package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	_ "io/ioutil"
	"net/http"
	"runtime"
	"time"
)

//
// Decode `reader` into the object `v`, and close `reader` after.
//
//
func decodeJSON(reader io.ReadCloser, v interface{}) error {
	var err = json.NewDecoder(reader).Decode(v)
	reader.Close()
	if err != nil {
		return fmt.Errorf("Couldn't decode JSON document: %s", err)
	}

	return nil
}

func encodeJSON(w io.Writer, v interface{}) error {
	var err = json.NewEncoder(w).Encode(v)
	if err != nil {
		return fmt.Errorf("Couldn't encode JSON document: %s", err)
	}

	return nil
}

//
// Query `baseURL/versions.json` for a new version of Sauce Connect
//
// Return the newest build number for the platform as determined by
// runtime.GOOS, and the URL to download the latest verion.
//
func GetLastVersion(baseUrl string, transport *http.Transport) (
	build int, url string, err error,
) {
	var client = http.Client{Transport: transport}
	var fullUrl = fmt.Sprintf("%s/versions.json", baseUrl)

	resp, err := client.Get(fullUrl)
	if err != nil {
		err = fmt.Errorf("Couldn't connect to %s: %s", fullUrl, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("Couldn't find %s: %s", fullUrl, resp.Status)
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
// Used to connect the SauceLabs REST API
//
type RequestConfig struct {
	BaseURL   string
	Username  string
	Password  string
	Transport *http.Transport
}

//
// Execute HTTP request and return an io.ReadCloser to be decoded
//
func executeRequest(req *http.Request, r *RequestConfig) (io.ReadCloser, error) {
	req.SetBasicAuth(r.Username, r.Password)

	var client = http.Client{Transport: r.Transport}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Couldn't connect to %s: %s", req.URL, err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("Couldn't find %s: %s", req.URL, resp.Status)
	}

	return resp.Body, nil
}

type TunnelStates []struct {
	Id               string   `json:"id"`
	TunnelIdentified string   `json:"tunnel_id"`
	DomainNames      []string `json:"domain_names"`
}

func GetTunnelStates(r *RequestConfig) (states TunnelStates, err error) {
	var url = fmt.Sprintf("%s/%s/tunnels?full=1", r.BaseURL, r.Username)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}

	body, err := executeRequest(req, r)
	if err != nil {
		return
	}

	err = decodeJSON(body, &states)
	if err != nil {
		return
	}

	return
}

//
// Return all the tunnels with the same id, tunnelId
//
func (self *TunnelStates) Match(tunnelId string, domains []string) TunnelStates {
	var newStates = TunnelStates{}

	for _, state := range *self {
		if state.TunnelIdentified == tunnelId {
			newStates = append(newStates, state)
			continue
		}

		for _, localDomain := range domains {
			for _, remoteDomain := range state.DomainNames {
				if localDomain == remoteDomain {
					newStates = append(newStates, state)
					continue
				}
			}
		}
	}

	return newStates
}

func removeTunnel(urlFmt, id string, config *RequestConfig) error {
	var url = fmt.Sprintf(urlFmt, config.BaseURL, config.Username, id)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	_, err = executeRequest(req, config)
	if err != nil {
		return err
	}

	return nil
}

// FIXME return the number of jobs running
func RemoveTunnel(id string, config *RequestConfig) error {
	return removeTunnel("%s/%s/tunnels/%s", id, config)
}

func RemoveTunnelForcefully(id string, config *RequestConfig) error {
	return removeTunnel("%s/%s/tunnels/%s?wait_for_jobs=1", id, config)
}

type Metadata struct {
	Release     string `json:"release"`
	GitVersion  string `json:"git_version"`
	Build       string `json:"build"`
	Platform    string `json:"platform"`
	Hostname    string `json:"hostname"`
	NoFileLimit int    `json:"no_file_limit"`
	Command     string `json:"command"`
}

type CreateRequest struct {
	DomainNames      []string  `json:"domain_names"`
	TunnelIdentifier *string   `json:"tunnel_identifier"`
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
}

func (self *CreateRequest) Execute(config *RequestConfig) (id string, err error) {
	var url = fmt.Sprintf("%s/%s/tunnels", config.BaseURL, config.Username)
	var jsonDoc bytes.Buffer
	if err = encodeJSON(&jsonDoc, self); err != nil {
		return
	}

	req, err := http.NewRequest("POST", url, &jsonDoc)
	if err != nil {
		return
	}

	body, err := executeRequest(req, config)
	if err != nil {
		return
	}

	var response struct {
		Id string
	}

	err = decodeJSON(body, &response)
	if err != nil {
		return
	}
	id = response.Id

	return
}

type tunnelStatus struct {
	Status       string
	UserShutdown bool `json:"user_shutdown"`
}

func getTunnelStatus(id string, config *RequestConfig) (
	status tunnelStatus, err error,
) {
	var url = fmt.Sprintf("%s/%s/tunnels/%s", config.BaseURL, config.Username, id)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}

	body, err := executeRequest(req, config)
	if err != nil {
		return
	}

	if err = decodeJSON(body, &status); err != nil {
		return
	}

	return
}

const (
	RUNNING = iota
	USER_SHUTDOWN
	TERMINATED
)

func isTunnelTerminated(id string, config *RequestConfig) (
	status int, err error,
) {
	s, err := getTunnelStatus(id, config)
	if err != nil {
		return
	}

	if s.UserShutdown {
		status = USER_SHUTDOWN
	} else if s.Status != "running" {
		status = TERMINATED
	} else {
		status = RUNNING
	}

	return
}

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

	body, err := executeRequest(req, config)
	if err != nil {
		return err
	}

	var response struct {
		Id string
	}

	err = decodeJSON(body, &response)
	if err != nil {
		return err
	}
	id = response.Id

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
