package main

import (
	"encoding/json"
	"bytes"
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
	var o struct {
		CommonOptions
		CheckVersion struct{} `command:"checkversion"`
	}
	parser := flags.NewParser(&o, flags.Default)
	extra, err := parser.ParseArgs(os.Args[1:])
	fmt.Printf("%#v\n", o)

	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if len(extra) != 0 {
		fmt.Fprintln(os.Stderr, "Extra arguments: ", extra)
		os.Exit(1)
	}

	var client = rest.Client{
		BaseURL:  o.RestUrl,
		// FIXME rename those in the rest lib later
		Username:     o.User,
		Password: o.ApiKey,
	}
	if len(o.Verbose) > 0 {
		client.DecodeJSON = verboseDecodeJSON
		client.EncodeJSON = verboseEncodeJSON
	}
	fmt.Printf("%#v\n", parser.Active.Name)
	switch parser.Active.Name {
	case "checkversion":
		build, u, err := client.GetLastVersion()
		if err == nil {
			fmt.Printf("%d %s\n", build, u)
		} else {
			fmt.Fprintf(
				os.Stderr,
				"error while check lastest version: %v\n",
				err)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", parser.Active.Name)
	}
}
