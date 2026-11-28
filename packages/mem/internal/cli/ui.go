package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"mem/internal/env"
	"mem/internal/store"
	"mem/internal/ui"
	"mem/internal/vec"
)

func cmdUI(e env.Env, args []string) int {
	f := newFlags(&e, "ui", "[--listen ADDR] [--allow-remote] [--state-dir D] [--relayout]", false)
	listen := f.String("listen", ui.DefaultListen, "address to listen on, loopback only unless --allow-remote")
	remote := f.Bool("allow-remote", false, "accept a non-loopback listen address")
	state := f.String("state-dir", e.StateDir(), "hook state directory read for turns.jsonl, shadow.jsonl and hook.log (the live feed)")
	relayout := f.Bool("relayout", false, "discard the stored graph layout and lay the map out again (it is kept per database in --state-dir)")
	pos, code, ok := f.parse(args)
	if !ok {
		return code
	}
	if len(pos) != 0 {
		_, _ = fmt.Fprintln(e.Stderr, "mem ui: takes no positional arguments")
		return 2
	}
	if err := ui.CheckListen(*listen, *remote); err != nil {
		return fail(e, fmt.Errorf("%w: ui: %v", store.ErrInvalid, err))
	}
	s, err := store.Open(context.Background(), e.DBPath(), store.Options{Now: e.Now, ReadOnly: true, Access: store.Access{All: true}})
	if err != nil {
		if store.IsNotExist(err) {
			return fail(e, fmt.Errorf("no memory database at %s; mem put creates it", e.DBPath()))
		}
		return fail(e, err)
	}
	defer func() { _ = s.Close() }()
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		return fail(e, err)
	}
	if err := ui.CheckBound(ln, *remote); err != nil {
		_ = ln.Close()
		return fail(e, err)
	}
	srv := ui.New(s, ui.Options{Host: e.Host(), Home: e.Home(), Now: e.Now, StateDir: *state, Relayout: *relayout,
		EmbedURL: e.Getenv("MEM_EMBED_URL"), EmbedModel: e.Getenv("MEM_EMBED_MODEL"), EmbedQueryPrefix: vec.ExpandEscapes(queryPrefixDefault(e)),
		EmbedDocPrefix: vec.ExpandEscapes(docPrefixDefault(e)), RerankURL: e.Getenv("MEM_RERANK_URL")}, *remote)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	_, _ = fmt.Fprintf(e.Stderr, "mem ui: http://%s/ serving %s read-only\n", ln.Addr(), s.Path())
	err = ui.Serve(ctx, ln, srv.Handler())
	srv.FlushLayout()
	if err != nil && !errors.Is(err, context.Canceled) {
		return fail(e, err)
	}
	return 0
}
