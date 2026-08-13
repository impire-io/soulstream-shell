// Command soulstream-shell serves the shell standalone — for realms running the
// components without soulnode. Every flag points at surfaces the
// deployment already runs; the shell founds and owns nothing.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/impire-io/soulstream-shell/embed"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version)
		return
	}
	var o embed.Options
	flag.StringVar(&o.Listen, "listen", "127.0.0.1:8500", "loopback HTTP address for the shell surface")
	flag.StringVar(&o.NATSURL, "nats", "nats://127.0.0.1:4222", "realm server URL")
	flag.StringVar(&o.CredsPath, "creds", "", "shell read-lane creds file")
	flag.StringVar(&o.CredsUser, "creds-user", "", "principal name of the read-lane creds")
	flag.StringVar(&o.SentinelPath, "sentinel", "", "public sentinel creds file for session admission")
	flag.StringVar(&o.Realm, "realm", "home", "realm name")
	flag.StringVar(&o.Account, "account", "", "realm account public key")
	flag.StringVar(&o.Issuer, "issuer", "", "OIDC authorization server (e.g. the fold)")
	flag.Parse()
	o.Ready = func(addr string) { log.Printf("soulstream-shell: shell console http://%s", addr) }

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := embed.Run(ctx, o); err != nil {
		log.Fatal(err)
	}
}
