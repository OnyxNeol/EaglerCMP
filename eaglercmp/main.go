// Command eaglercmp is the EaglerCMP desktop client. It installs an
// Eaglercraft web client locally, serves it from a loopback HTTP server and
// opens it in a dedicated desktop window tuned by the Sodium HX Graphics
// (NaOHX) performance profile. No accounts, no Mojang downloads.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/OnyxNeol/eaglercmp/bundle"
	"github.com/OnyxNeol/eaglercmp/config"
	"github.com/OnyxNeol/eaglercmp/downloader"
	"github.com/OnyxNeol/eaglercmp/manager"
)

const usage = `EaglerCMP - EaglerCraft with Mods and Performance
Performance engine: Sodium HX Graphics (NaOHX)

Usage:
  eaglercmp [command] [flags] [args]

Commands:
  launch            Verify the client, serve it locally and open the window (default)
  import <source>   Install an Eaglercraft web client from a .html file, folder,
                    .zip/.tar.gz archive, or http(s) URL
  init              Create the instance and install the client (clientSource,
                    else the bundled client, else the upstream 26.2 repo)
  update            Re-download the client from clientSource or the upstream repo
  java-client       Run the real Minecraft 26.2 Java client (Fabric Loader, offline
                    session) in a local JVM; put Fabric client mods in
                    <instance>/javaclient/game/mods
  status            Show the instance, client and window configuration
  credits           Show open-source attribution

Flags:
`

// gui is true when the Windows exe was double-clicked: there is no console,
// so output goes to the log files and failures are shown in a dialog.
var gui bool

// lastLog is the launcher logger of the current run, for the error dialog.
var lastLog *manager.Logger

func main() {
	gui = setupConsole()
	code := run(os.Args[1:])
	if code != 0 && gui {
		msg := "EaglerCMP stopped with an error."
		if lastLog != nil {
			if l := lastLog.Last(); l != "" {
				msg = l
			}
			msg += "\n\nLog: " + lastLog.Path()
		}
		showDialog(config.WindowTitle(), msg, true)
	}
	os.Exit(code)
}

func run(argv []string) int {
	cmd := "launch"
	if len(argv) > 0 && !strings.HasPrefix(argv[0], "-") {
		cmd, argv = argv[0], argv[1:]
	}

	fs := flag.NewFlagSet("eaglercmp", flag.ContinueOnError)
	dir := fs.String("dir", config.DefaultRoot(), "instance directory (env EAGLERCMP_HOME)")
	port := fs.Int("port", 0, "override the loopback port for this run")
	browser := fs.String("browser", "", `override the window browser ("system" = default browser)`)
	sum := fs.String("sha256", "", "import: expected SHA-256 of the source file")
	dryRun := fs.Bool("dry-run", false, "launch: print the window command instead of running it")
	noWindow := fs.Bool("no-window", false, "launch: only serve the client; open the URL yourself")
	skipVerify := fs.Bool("skip-verify", false, "launch: skip re-hashing client files")
	username := fs.String("username", "", "java-client: offline player name (3-16 letters/digits/_)")
	verbose := fs.Bool("verbose", false, "launch: show every browser-engine log line")
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), usage)
		fs.PrintDefaults()
	}
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		fs.Usage()
		return 0
	}
	if err := fs.Parse(argv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if cmd == "credits" {
		config.PrintCredits(os.Stdout)
		return 0
	}

	root, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	paths := config.NewPaths(root)
	var console io.Writer = os.Stdout
	if gui {
		console = io.Discard
	}
	log, err := manager.NewLogger(console, paths.Logs)
	if err != nil {
		if gui {
			showDialog(config.WindowTitle(), err.Error(), true)
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer log.Close()
	lastLog = log

	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	if *port != 0 {
		cfg.Port = *port
	}
	if *browser != "" {
		cfg.Browser = *browser
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt := manager.New(cfg, paths, log)
	switch cmd {
	case "import":
		if fs.NArg() != 1 {
			fmt.Fprintln(os.Stderr, "usage: eaglercmp import [-sha256 HEX] <file.html|dir|archive|url>")
			return 2
		}
		if _, err := rt.Import(ctx, fs.Arg(0), *sum); err != nil {
			log.Printf("import failed: %v", err)
			return 1
		}
		return 0

	case "update":
		if _, err := rt.Update(ctx); err != nil {
			log.Printf("update failed: %v", err)
			return 1
		}
		return 0

	case "init":
		config.Splash(log.Writer())
		if _, err := rt.Prepare(ctx, false); err != nil {
			log.Printf("init: %v", err)
			return 1
		}
		log.Printf("instance ready in %s", root)
		return 0

	case "launch":
		config.Splash(log.Writer())
		m, err := rt.Prepare(ctx, *skipVerify)
		if errors.Is(err, manager.ErrNoClient) && interactive() {
			var src string
			if src, err = pickClient(ctx); err == nil {
				m, err = rt.Import(ctx, src, "")
			}
		}
		if err != nil {
			log.Printf("%v", err)
			return 1
		}
		if *dryRun {
			bin, args, err := rt.Command()
			if err != nil {
				log.Printf("%v", err)
				return 1
			}
			fmt.Println(strings.Join(append([]string{bin}, args...), " "))
			return 0
		}
		code, err := rt.Launch(ctx, m, manager.LaunchOptions{NoWindow: *noWindow, Verbose: *verbose}, console)
		if err != nil {
			log.Printf("launch failed: %v", err)
			return 1
		}
		return code

	case "java-client":
		jc := cfg.JavaClient
		if *username != "" {
			jc.Username = *username
		}
		srv := "127.0.0.1:25565"
		if cfg.Gateway.Enabled && cfg.Gateway.Port != 0 {
			srv = fmt.Sprintf("127.0.0.1:%d", cfg.Gateway.Port)
		} else if !cfg.Gateway.Enabled && cfg.Backend.Enabled {
			srv = "127.0.0.1:25566"
			if cfg.Backend.Port != 0 {
				srv = fmt.Sprintf("127.0.0.1:%d", cfg.Backend.Port)
			}
		}
		runner := &manager.JavaClient{Cfg: jc, Root: root, Server: srv, Log: log}
		code, err := runner.Run(ctx, *dryRun, console)
		if err != nil {
			log.Printf("java-client: %v", err)
			return 1
		}
		return code

	case "status":
		return status(rt)
	}

	fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
	fs.Usage()
	return 2
}

func status(rt *manager.Runtime) int {
	cfg, paths := rt.Cfg, rt.Paths
	fmt.Printf("Instance:  %s\n", paths.Root)
	if m, err := downloader.ReadManifest(paths.Client); err == nil {
		state := "verified"
		if _, err := downloader.VerifyClient(paths.Client); err != nil {
			state = "MODIFIED: " + err.Error()
		}
		fmt.Printf("Client:    %s (%d files, %.1f MB, %s)\n", m.Entry, len(m.Files), float64(m.Bytes)/(1<<20), state)
		fmt.Printf("Source:    %s\n", m.Source)
	} else {
		fmt.Printf("Client:    not installed\n")
	}
	if len(bundle.Archive()) > 0 {
		fmt.Printf("Bundled:   %.1f MB (%s)\n", float64(len(bundle.Archive()))/(1<<20), bundle.Source())
	}
	fmt.Printf("Served at: %s\n", rt.URL())
	if cfg.Browser == manager.SystemBrowser {
		fmt.Printf("Window:    default browser\n")
	} else if bin, err := manager.FindBrowser(cfg.Browser); err == nil {
		fmt.Printf("Window:    %s (%dx%d)\n", bin, cfg.Width, cfg.Height)
	} else {
		fmt.Printf("Window:    %v (falls back to the default browser)\n", err)
	}
	fmt.Printf("%s:     %v\n", config.EngineShort, cfg.Performance)
	for _, s := range cfg.Servers {
		fmt.Printf("Server:    %-20s %s\n", s.Name, s.Addr)
	}
	return 0
}
