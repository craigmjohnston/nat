package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/plugins"
)

// NewPluginsFunc builds the plugin installer the plugin-* commands run
// through.
type NewPluginsFunc func() (*plugins.Manager, error)

// DefaultNewPlugins is the installer over GitHub itself, installing under
// nat's own config directory.
func DefaultNewPlugins() (*plugins.Manager, error) {
	dir, err := config.Dir()
	if err != nil {
		return nil, err
	}
	return plugins.New(dir), nil
}

// pluginConfig is the config the plugin commands read their sources and
// projects from. A machine with no config yet has neither: nat's own source,
// and no project to be using a plugin.
func pluginConfig(env Env) (config.Config, error) {
	cfg, _, err := env.Load()
	return cfg, err
}

// pluginList prints every plugin source, every installed plugin and every
// plugin a source offers. A source that cannot be read is listed with its
// error, not as one offering nothing.
func pluginList(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("plugin-list", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of plain text")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("plugin-list: takes no arguments, given %d", len(rest))
	}
	cfg, err := pluginConfig(env)
	if err != nil {
		return err
	}
	m, err := env.NewPlugins()
	if err != nil {
		return err
	}
	listing, err := m.List(ctx, plugins.Sources(cfg))
	if err != nil {
		return fmt.Errorf("look for task source plugins: %w", err)
	}
	if *asJSON {
		return writeJSON(env.Out, listing)
	}
	_, err = io.WriteString(env.Out, pluginListText(listing))
	return err
}

// pluginListText is plugin-list's plain form.
func pluginListText(l plugins.Listing) string {
	var b strings.Builder
	b.WriteString("sources:\n")
	for _, s := range l.Sources {
		if s.Error != "" {
			fmt.Fprintf(&b, "  %s\terror: %s\n", s.Repo, s.Error)
			continue
		}
		fmt.Fprintf(&b, "  %s\t%s\n", s.Repo, s.Version)
	}
	b.WriteString("installed:\n")
	for _, p := range l.Installed {
		fmt.Fprintf(&b, "  %s\t%s\t%s", p.Name, p.Kind, p.Path)
		if p.Version != "" {
			fmt.Fprintf(&b, "\t%s from %s", p.Version, p.Source)
		}
		if p.Update != "" {
			fmt.Fprintf(&b, "\t(update: %s)", p.Update)
		}
		b.WriteString("\n")
	}
	b.WriteString("available:\n")
	for _, p := range l.Available {
		fmt.Fprintf(&b, "  %s\t%s %s\t%s", p.Name, p.Source, p.Version, p.Title)
		if p.Installed {
			b.WriteString("\t(installed)")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// pluginInstall installs, or updates, one plugin from a source: the named
// one, else the first that offers it, nat's own repository first.
func pluginInstall(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("plugin-install", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	from := flags.String("source", "", "install from this plugin source, `owner/repo`")
	version := flags.String("version", "", "install this release `version` instead of the latest")
	asJSON := flags.Bool("json", false, "print structured JSON instead of plain text")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("plugin-install: want exactly one plugin, by name")
	}
	cfg, err := pluginConfig(env)
	if err != nil {
		return err
	}
	m, err := env.NewPlugins()
	if err != nil {
		return err
	}
	rec, err := m.Install(ctx, plugins.Sources(cfg), rest[0], strings.TrimSpace(*from), strings.TrimPrefix(strings.TrimSpace(*version), "v"))
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(env.Out, rec)
	}
	_, err = fmt.Fprintf(env.Out, "Installed %s %s from %s at %s.\n", rec.Name, rec.Version, rec.Source, rec.Path)
	return err
}

// pluginUninstalledJSON is what plugin-uninstall took away.
type pluginUninstalledJSON struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// pluginUninstall removes one plugin from nat's plugins directory, refused
// while a project is a source project of it.
func pluginUninstall(args []string, env Env) error {
	flags := flag.NewFlagSet("plugin-uninstall", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of plain text")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("plugin-uninstall: want exactly one plugin, by name")
	}
	cfg, err := pluginConfig(env)
	if err != nil {
		return err
	}
	m, err := env.NewPlugins()
	if err != nil {
		return err
	}
	dir, err := m.Uninstall(rest[0], cfg)
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(env.Out, pluginUninstalledJSON{Name: rest[0], Path: dir})
	}
	_, err = fmt.Fprintf(env.Out, "Uninstalled %s from %s.\n", rest[0], dir)
	return err
}

// pluginSourcesJSON is every plugin source after a change, in reading order.
type pluginSourcesJSON struct {
	Sources []string `json:"sources"`
}

// pluginSourceAdd adds one plugin source to the config, after nat's own.
func pluginSourceAdd(args []string, env Env) error {
	return pluginSourceEdit("plugin-source-add", args, env, func(cfg *config.Config, repo string) error {
		if slices.Contains(plugins.Sources(*cfg), repo) {
			return fmt.Errorf("plugin-source-add: %s is already a plugin source", repo)
		}
		cfg.PluginSources = append(cfg.PluginSources, repo)
		return nil
	})
}

// pluginSourceRemove takes one plugin source out of the config. nat's own is
// not the config's to remove.
func pluginSourceRemove(args []string, env Env) error {
	return pluginSourceEdit("plugin-source-remove", args, env, func(cfg *config.Config, repo string) error {
		if repo == plugins.DefaultSource {
			return fmt.Errorf("plugin-source-remove: %s is nat's own plugin source and is always read", repo)
		}
		i := slices.Index(cfg.PluginSources, repo)
		if i < 0 {
			return fmt.Errorf("plugin-source-remove: %s is not a plugin source", repo)
		}
		cfg.PluginSources = slices.Delete(cfg.PluginSources, i, i+1)
		return nil
	})
}

// pluginSourceEdit is the two source commands' shared shape: one owner/repo,
// a config that exists, edit, save, print every source.
func pluginSourceEdit(command string, args []string, env Env, edit func(*config.Config, string) error) error {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of plain text")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("%s: want exactly one plugin source, as owner/repo", command)
	}
	repo := strings.TrimSpace(rest[0])
	if err := plugins.ValidRepo(repo); err != nil {
		return fmt.Errorf("%s: %w", command, err)
	}
	cfg, found, err := env.Load()
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("no configuration yet: run `nat` once to set it up")
	}
	if err := edit(&cfg, repo); err != nil {
		return err
	}
	if err := env.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	sources := plugins.Sources(cfg)
	if *asJSON {
		return writeJSON(env.Out, pluginSourcesJSON{Sources: sources})
	}
	_, err = io.WriteString(env.Out, strings.Join(sources, "\n")+"\n")
	return err
}
