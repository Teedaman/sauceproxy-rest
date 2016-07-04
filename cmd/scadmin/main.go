package main

import (
	"encoding/json"
	"fmt"
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

	err = json.NewDecoder(resp.Body).Decode(&jsonStruct)
	if err != nil {
		err = fmt.Errorf("Couldn't decode JSON document: %s", err)
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

func main() {
	var tr = &http.Transport{}

	build, url, err := GetLastVersion("https://saucelabs.com", tr)
	if err == nil {
		fmt.Printf("Latest build: %d: %s\n", build, url)
	} else {
		fmt.Printf("ERROR: %s\n", err)
	}
}
