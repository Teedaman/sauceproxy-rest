package main

import (
	"fmt"
	"github.com/saucelabs/sauceproxy-rest/admin"
	"net/http"
	"time"
)

func main() {
	// FIXME use this to configure proxy & TLS
	var tr = &http.Transport{}

	var err error

	/*
		build, url, err := admin.GetLastVersion("https://saucelabs.com", tr)
		if err == nil {
			fmt.Printf("Latest build: %d: %s\n", build, url)
		} else {
			fmt.Printf("ERROR: %s\n", err)
		}
	*/

	var config = &admin.RequestConfig{
		BaseURL:   "https://saucelabs.com/rest/v1",
		Username:  "henryprecheur",
		Password:  "fd698b0a-744c-4304-b1bd-16e2734127bf",
		Transport: tr,
	}

	/*
		info, err := admin.GetTunnelStates(config)
		if err == nil {
			fmt.Printf("return: %+v\n", info)
		} else {
			panic(err)
		}

		var matches = info.Match("foobar", []string{"sauce-connect.proxy"})

		fmt.Printf("%+v\n", matches)

		for _, m := range matches {
			fmt.Printf("Removing tunnel: %+v\n", m.Id)
			err := admin.RemoveTunnel(m.Id, config)
			if err != nil {
				panic(err)
			}
		}
	*/

	var req = admin.CreateRequest{
		DomainNames: []string{"sauce-connect.proxy"},
		Metadata: admin.Metadata{
			Release:     "1.2.3",
			GitVersion:  "4a804fd",
			Build:       "Strong",
			Platform:    "Plan9 bitch",
			Hostname:    "Henry's computer",
			NoFileLimit: 12345,
			Command:     "./sc",
		},
		SSHPort: 443,
		UseKGP:  true,
	}
	id, err := req.Execute(config)
	if err != nil {
		panic(err)
	}
	fmt.Printf("New tunnel id: %s\n", id)

	// var tid = "a84677529b3041e6ac15f9526dc171f8"
	fmt.Println(admin.WaitForTunnel(id, config))

	var delay = 10 * time.Second
	time.Sleep(delay)
	fmt.Println(admin.SendHeartBeat(id, true, delay, config))
	time.Sleep(delay)
	fmt.Println(admin.SendHeartBeat(id, true, delay, config))
	time.Sleep(delay)
	fmt.Println(admin.SendHeartBeat(id, true, delay, config))
}
