package main

import (
	"net/http"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/jessevdk/go-flags"
	"io"
	"log"
	"os"

	// rest "../.."
	rest "github.com/saucelabs/sauceproxy-rest"
)

type CommonOptions struct {
	User    string `short:"u" long:"user" value-name:"<username>" description:"The environment variable SAUCE_USERNAME can also be used." required:"yes" env:"SAUCE_USERNAME"`
	ApiKey  string `short:"k" long:"api-key" value-name:"<api-key>" description:"The environment variable SAUCE_ACCESS_KEY can also be used." required:"yes" env:"SAUCE_ACCESS_KEY"`
	RestUrl string `short:"x" long:"rest-url" value-name:"<arg>" description:"Advanced feature: Connect to Sauce REST API at alternative URL. Use only if directed to do so by Sauce Labs support." default:"https://saucelabs.com/rest/v1"`
	Help    bool   `short:"h" long:"help" description:"Show usage information."`
	Verbose []bool `short:"v" long:"verbose" description:"Enable verbose debugging."`
}

type TunnelOptions struct {
	TunnelIdentifier string   `short:"i" long:"tunnel-identifier" value-name:"<id>" description:"Don't automatically assign jobs to this tunnel. Jobs will use it only by explicitly providing the right identifier."`
	TunnelDomains    []string `short:"t" long:"tunnel-domains" value-name:"<...>" description:"Inverse of '--direct-domains'. Only requests for domains in this list will be sent through the tunnel. Overrides '--direct-domains'."`
}

type CreateOptions struct {
	TunnelOptions

	DirectDomains    []string `short:"D" long:"direct-domains" value-name:"<...>" description:"Comma-separated list of domains. Requests whose host matches one of these will be relayed directly through the internet, instead of through the tunnel."`
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
	logger.Println("response:", buf.String(), "\n")
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
	logger.Println("request:", buf, "\n")
	io.Copy(w, &buf)
	if err != nil {
		return fmt.Errorf("couldn't encode JSON document: %s", err)
	}

	return nil
}

type Options struct {
	CommonOptions
	CheckVersion struct{}      `command:"checkversion"`
	Create       CreateOptions `command:"create"`
	Shutdown     struct {
		Arg struct {
			Id string `description:"Tunnel ID (not tunnel identifier)"`
		} `positional-args:"yes" required:"yes"`
	} `command:"shutdown"`
	Status struct {
		Arg struct {
			Id string `description:"Tunnel ID (not tunnel identifier)"`
		} `positional-args:"yes" required:"yes"`
	} `command:"status"`
	Find TunnelOptions `command:"find"`
	List struct{} `command:"list"`
}

// Return the command name and the options object
//
// Exits if there's any error
func ParseArguments(args []string) (command string, options Options) {
	parser := flags.NewParser(&options, flags.Default)
	extra, err := parser.ParseArgs(args)

	if err != nil {
		// FIXME go-flags outputs the error in stderr in some cases, check it
		// does it for all errors
		os.Exit(1)
	}
	if len(extra) != 0 {
		logger.Fatalln("Extra arguments:", extra)
	}
	command = parser.Active.Name

	return
}

var logger *log.Logger

func init() {
	logger = log.New(os.Stderr, "", 0)
}

func main() {
	var command, o = ParseArguments(os.Args[1:])

	var httpclient = http.Client{
		Transport: &http.Transport{ Proxy: http.ProxyFromEnvironment },
	}
	var client = rest.Client{
		BaseURL: o.RestUrl,
		// FIXME rename those in the rest lib later
		Username: o.User,
		Password: o.ApiKey,

		Client: httpclient,
	}
	if len(o.Verbose) > 0 {
		client.DecodeJSON = verboseDecodeJSON
		client.EncodeJSON = verboseEncodeJSON
	}
	switch command {
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
			DomainNames:      options.TunnelDomains,
			DirectDomains:    options.DirectDomains,
			KGPPort:          options.KgpPort,
			NoProxyCaching:   options.NoProxyCaching,
			FastFailRegexps:  options.FastFailRegexps,
			SharedTunnel:     options.SharedTunnel,
			VMVersion:        options.VmVersion,
			NoSSLBumpDomains: options.NoSslBumpDomains,
			Command:          "sauceproxy-rest",
		})
		if err != nil {
			logger.Fatalln("Unable to create tunnel:", err)
		}
		logger.Println("Tunnel successfully created")
		fmt.Println(tunnel.Id)
	case "shutdown":
		var id = o.Shutdown.Arg.Id
		err := client.Shutdown(id)
		if err != nil {
			logger.Fatalln("Unable to shutdown tunnel:", err)
		}
		logger.Println("Tunnel", id, "shutting down.")
	case "status":
		var id = o.Status.Arg.Id
		status, err := client.Status(id)
		if err != nil {
			logger.Fatalln("Unable to shutdown tunnel:", err)
		}
		fmt.Println(status)
	case "find":
		var q = o.Find
		matches, err := client.Find(q.TunnelIdentifier, q.TunnelDomains)
		if err != nil {
			log.Fatalln(err)
		}
		for _, id := range matches {
			fmt.Println(id)
		}
	case "list":
		matches, err := client.List()
		if err != nil {
			log.Fatalln(err)
		}
		for _, id := range matches {
			fmt.Println(id)
		}
	default:
		logger.Fatalln("unknown command:", command)
	}
}
