package main

import (
	"encoding/json"
	"fmt"
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

	resp, err := client.Get(fmt.Sprintf("%s/versions.json", baseUrl))
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var decoder = json.NewDecoder(resp.Body)

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

	err = decoder.Decode(&jsonStruct)
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

func main() {
	var tr = &http.Transport{}

	build, url, err := GetLastVersion("https://saucelabs.com", tr)
	if err == nil {
		fmt.Printf("Latest build: %d: %s\n", build, url)
	} else {
		fmt.Printf("ERROR: %s\n", err)
	}
}
