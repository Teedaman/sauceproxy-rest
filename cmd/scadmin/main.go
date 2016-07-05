package main

import (
	"fmt"
	"net/http"
	"github.com/saucelabs/sauceproxy-rest/admin"
)

func main() {
	// FIXME use this to configure proxy & TLS
	var tr = &http.Transport{}

	build, url, err := admin.GetLastVersion("https://saucelabs.com", tr)
	if err == nil {
		fmt.Printf("Latest build: %d: %s\n", build, url)
	} else {
		fmt.Printf("ERROR: %s\n", err)
	}

	var config = &admin.RequestConfig{
		BaseURL:   "https://saucelabs.com/rest/v1",
		Username:  "henryprecheur",
		Password:  "fd698b0a-744c-4304-b1bd-16e2734127bf",
		Transport: tr,
	}

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
}
