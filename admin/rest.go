package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
)

//
// Decode `reader` into the object `v`, and close `reader` after.
//
//
func decodeJSON(reader io.ReadCloser, v interface{}) error {
	defer reader.Close()
	var err = json.NewDecoder(reader).Decode(v)
	if err != nil {
		return fmt.Errorf("Couldn't decode JSON document: %s", err)
	}

	return nil
}

//
// Query `baseURL/versions.json` for a new version of Sauce Connect
//
// Return the newest build number for the platform as determined by
// runtime.GOOS, and the URL to download the latest verion.
//
func GetLastVersion(baseUrl string, transport *http.Transport) (build int, url string, err error) {
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
