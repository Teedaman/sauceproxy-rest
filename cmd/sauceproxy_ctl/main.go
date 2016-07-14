package main

import (
	"log"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/jessevdk/go-flags"
	"io"
	"os"

	// "github.com/saucelabs/sauceproxy-rest"
	rest "../.."
)

type CommonOptions struct {
	User    string `short:"u" long:"user" value-name:"<username>" description:"The environment variable SAUCE_USERNAME can also be used."`
	ApiKey  string `short:"k" long:"api-key" value-name:"<api-key>" description:"The environment variable SAUCE_ACCESS_KEY can also be used."`
	RestUrl string `short:"x" long:"rest-url" value-name:"<arg>" description:"Advanced feature: Connect to Sauce REST API at alternative URL. Use only if directed to do so by Sauce Labs support." default:"https://saucelabs.com/rest/v1"`
	Help    bool   `short:"h" long:"help" description:"Show usage information."`
	Verbose []bool `short:"v" long:"verbose" description:"Enable verbose debugging."`
}

type TunnelOptions struct {
	TunnelIdentifier string   `short:"i" long:"tunnel-identifier" value-name:"<id>" description:"Don't automatically assign jobs to this tunnel. Jobs will use it only by explicitly providing the right identifier."`
	TunnelDomains    []string `short:"t" long:"tunnel-domains" value-name:"<...>" description:"Inverse of '--direct-domains'. Only requests for domains in this list will be sent through the tunnel. Overrides '--direct-domains'."`
	DirectDomains    []string `short:"D" long:"direct-domains" value-name:"<...>" description:"Comma-separated list of domains. Requests whose host matches one of these will be relayed directly through the internet, instead of through the tunnel."`
}

type CreateOptions struct {
	TunnelOptions

	NoProxyCaching   bool     `short:"N" long:"no-proxy-caching" description:"Disable caching in Sauce Connect. All requests will be sent through the tunnel."`
	KgpPort          int      `long:"kgp-port" hidden:"true" default:"443"`
	FastFailRegexps  []string `short:"F" long:"fast-fail-regexps" value-name:"<...>" description:"Comma-separated list of regular expressions. Requests matching one of these will get dropped instantly and will not go through the tunnel."`
	SharedTunnel     bool     `short:"s" long:"shared-tunnel" description:"Let sub-accounts of the tunnel owner use the tunnel if requested."`
	VmVersion        string   `long:"vm-version" value-name:"<version>" description:"Request a specific tunnel VM version."`
	NoSslBumpDomains []string `short:"B" long:"no-ssl-bump-domains" value-name:"<...>" description:"Comma-separated list of domains. Requests whose host matches one of these will not be SSL re-encrypted."`
}

//
// Decode `reader` into the object `v`, and close `reader` after.
//
//
func verboseDecodeJSON(reader io.ReadCloser, v interface{}) error {
	var buf bytes.Buffer
	io.Copy(&buf, reader)
	fmt.Fprintf(os.Stderr, "response:\n%s\n\n", buf)
	var err = json.NewDecoder(&buf).Decode(v)
	reader.Close()
	if err != nil {
		return fmt.Errorf("couldn't decode JSON document: %s", err)
	}

	return nil
}

func verboseEncodeJSON(w io.Writer, v interface{}) error {
	var buf bytes.Buffer
	var err = json.NewEncoder(&buf).Encode(v)
	fmt.Fprintf(os.Stderr, "request:\n%s\n\n", buf)
	io.Copy(w, &buf)
	if err != nil {
		return fmt.Errorf("couldn't encode JSON document: %s", err)
	}

	return nil
}

func main() {
	logger := log.New(os.Stderr, "", 0)
	var o struct {
		CommonOptions
		CheckVersion struct{}      `command:"checkversion"`
		Create       CreateOptions `command:"create"`
	}
	parser := flags.NewParser(&o, flags.Default)
	extra, err := parser.ParseArgs(os.Args[1:])

	if err != nil {
		os.Exit(1)
	}
	if len(extra) != 0 {
		logger.Fatalln("Extra arguments:", extra)
	}

	var client = rest.Client{
		BaseURL: o.RestUrl,
		// FIXME rename those in the rest lib later
		Username: o.User,
		Password: o.ApiKey,
	}
	if len(o.Verbose) > 0 {
		client.DecodeJSON = verboseDecodeJSON
		client.EncodeJSON = verboseEncodeJSON
	}
	switch parser.Active.Name {
	case "checkversion":
		build, u, err := client.GetLastVersion()
		if err == nil {
			fmt.Printf("%d %s\n", build, u)
		} else {
			logger.Fatalln("Error checking lastest version:", err)
		}
	case "create":
		var options = o.Create

		tunnel, err := client.Create(&rest.Request{
			TunnelIdentifier: options.TunnelIdentifier,
			DomainNames: options.TunnelDomains,
			DirectDomains: options.DirectDomains,
			KGPPort: options.KgpPort,
			NoProxyCaching: options.NoProxyCaching,
			FastFailRegexps: options.FastFailRegexps,
			SharedTunnel: options.SharedTunnel,
			VMVersion: options.VmVersion,
			NoSSLBumpDomains: options.NoSslBumpDomains,
			Command: "sauceproxy-rest",
		})
		if err != nil {
			logger.Fatalln("Unable to create tunnel:", err)
		}
		_ = tunnel
		fmt.Fprintln(os.Stderr, "Tunnel successfully created")
		fmt.Println(tunnel.Id)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", parser.Active.Name)
	}
}
