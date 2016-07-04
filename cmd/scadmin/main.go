package main

import (
	"encoding/json"
	"fmt"
	"io"
	_ "io/ioutil"
	"net/http"
	"runtime"
)

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

	// Structure we use to decode the json document
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

	err = DecodeJSON(resp.Body, &jsonStruct)
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

type RequestConfig struct {
	BaseURL   string
	Username  string
	Password  string
	Transport *http.Transport
}

//
// Execute HTTP request and return an io.ReadCloser to be decoded
//
func ExecuteRequest(req *http.Request, r *RequestConfig) (io.ReadCloser, error) {
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

//
// This will close `reader`
//
func DecodeJSON(reader io.ReadCloser, v interface{}) error {
	defer reader.Close()
	var err = json.NewDecoder(reader).Decode(v)
	if err != nil {
		return fmt.Errorf("Couldn't decode JSON document: %s", err)
	}

	return nil
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

	body, err := ExecuteRequest(req, r)
	if err != nil {
		return
	}

	err = DecodeJSON(body, &states)
	if err != nil {
		return
	}

	return
}

//
// Return all the tunnels with the same id, tunnelId
//
func (self *TunnelStates) Match(tunnelId string, domains []string) TunnelStates {
	var newStates TunnelStates

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

func main() {
	var tr = &http.Transport{}

	build, url, err := GetLastVersion("https://saucelabs.com", tr)
	if err == nil {
		fmt.Printf("Latest build: %d: %s\n", build, url)
	} else {
		fmt.Printf("ERROR: %s\n", err)
	}

	var config = &RequestConfig{
		BaseURL:   "https://saucelabs.com/rest/v1",
		Username:  "henryprecheur",
		Password:  "fd698b0a-744c-4304-b1bd-16e2734127bf",
		Transport: tr,
	}

	info, err := GetTunnelStates(config)
	if err == nil {
		fmt.Printf("return: %+v\n", info)
	} else {
		panic(err)
	}
	fmt.Printf("%+v\n", info.Match("foobar", []string{"sauce-connect.proxy"}))
}
