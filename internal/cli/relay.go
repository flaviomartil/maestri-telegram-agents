package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/permgps/herdr-telegram-agents/internal/compose"
	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

func RelayMain(args []string, in io.Reader, out, errOut io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := runRelayCLI(ctx, args, in, out, errOut); err != nil {
		if errors.Is(err, context.Canceled) {
			return 0
		}
		fmt.Fprintln(errOut, err)
		return 1
	}
	return 0
}

func runRelayCLI(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintln(out, "maestri-tg <init|pair|doctor|catalog-sync|catalog-list|catalog-export|plan|apply|run> [--config PATH]\nSegredos: TELEGRAM_BOT_TOKEN, MAESTRI_WIRE_TOKEN, MAESTRI_LLM_KEY. Use <comando> --help para opções.")
		return nil
	}
	command := args[0]
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	f.SetOutput(errOut)
	configPath := f.String("config", filepath.Join(home, ".config", "maestri-relay", "config.json"), "configuração JSON")
	output := f.String("out", "", "arquivo de saída")
	request := f.String("request", "", "descrição para o Maestro")
	template := f.String("template", "", "ID do exemplo base")
	workspace := f.String("workspace", "", "ID do workspace existente")
	floor := f.String("floor", "", "ID do andar existente; vazio é térreo")
	planPath := f.String("plan", "", "plano JSON a aplicar")
	job := f.String("job", "", "ID estável desta criação")
	query := f.String("query", "", "filtrar catálogo por nome ou descrição")
	if err = f.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return errors.New("argumentos extras não reconhecidos")
	}
	if command == "init" {
		cfg := domain.RelayConfig{WireURL: "https://127.0.0.1:7434", StateDir: filepath.Join(home, ".local", "state", "maestri-relay"), CatalogDir: filepath.Join(home, ".cache", "maestri-relay", "guide"), PollSeconds: 5, Workspaces: []string{}, Operators: []int64{}}
		b, _ := json.MarshalIndent(cfg, "", "  ")
		if err = os.MkdirAll(filepath.Dir(*configPath), 0700); err != nil {
			return err
		}
		file, e := os.OpenFile(*configPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		_, err = file.Write(b)
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		fmt.Fprintln(out, *configPath)
		return nil
	}
	cfg, err := loadRelayConfig(*configPath)
	if err != nil {
		return err
	}
	if command != "run" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
	}
	switch command {
	case "pair":
		if cfg.WirePin == "" {
			return errors.New("configure wirePin com a chave de segurança exibida pelo host antes de parear")
		}
		wire, e := compose.RelayWire(cfg)
		if e != nil {
			return e
		}
		fmt.Fprint(out, "Código de pareamento do Maestri: ")
		scanner := bufio.NewScanner(in)
		if !scanner.Scan() {
			return errors.New("código não informado")
		}
		token, e := wire.Pair(ctx, strings.TrimSpace(scanner.Text()))
		if e != nil {
			return e
		}
		if e = compose.RelayWrite(filepath.Join(cfg.StateDir, "wire-token"), []byte(token)); e != nil {
			return e
		}
		fmt.Fprintln(out, "Pareamento salvo.")
		return nil
	case "doctor":
		wire, e := compose.RelayWire(cfg)
		if e != nil {
			return e
		}
		info, e := wire.Info(ctx)
		if e != nil {
			return e
		}
		if info.Role == "" {
			return errors.New("resposta Wire recebida, mas o pareamento está ausente ou inválido")
		}
		b, _ := json.MarshalIndent(info, "", "  ")
		fmt.Fprintln(out, string(b))
		return nil
	case "catalog-sync":
		if err = compose.RelaySync(ctx, cfg); err != nil {
			return err
		}
		c, e := compose.RelayCatalog(cfg)
		if e != nil {
			return e
		}
		fmt.Fprintf(out, "Catálogo sincronizado: %d entradas.\n", len(c.List()))
		return nil
	case "catalog-list":
		c, e := compose.RelayCatalog(cfg)
		if e != nil {
			return e
		}
		for _, a := range c.List() {
			if *query == "" || strings.Contains(strings.ToLower(a.Name+" "+a.Description), strings.ToLower(*query)) {
				fmt.Fprintf(out, "%s\t%s\n", a.ID, a.Name)
			}
		}
		return nil
	case "catalog-export":
		if *output == "" {
			return errors.New("informe --out para o pacote .maestripartituras")
		}
		b, e := compose.RelayExport(ctx, cfg)
		if e != nil {
			return e
		}
		if e = compose.RelayWrite(*output, b); e != nil {
			return e
		}
		fmt.Fprintln(out, *output)
		return nil
	case "plan":
		base := domain.MaestroPlan{WorkspaceID: *workspace, FloorID: *floor}
		if *template != "" {
			c, e := compose.RelayCatalog(cfg)
			if e != nil {
				return e
			}
			base.Arrangement, e = c.Get(*template)
			if e != nil {
				return e
			}
		}
		plan := base
		if *request != "" {
			plan, err = compose.RelayPlan(ctx, cfg, *request, base)
			if err != nil {
				return err
			}
		} else if *template == "" {
			return errors.New("informe --request ou --template")
		}
		b, e := json.MarshalIndent(plan, "", "  ")
		if e != nil {
			return e
		}
		if *output != "" {
			if e = compose.RelayWrite(*output, b); e != nil {
				return e
			}
			fmt.Fprintln(out, *output)
		} else {
			fmt.Fprintln(out, string(b))
		}
		return nil
	case "apply":
		if *planPath == "" || *job == "" {
			return errors.New("informe --plan e --job; mantenha o mesmo job ao retomar")
		}
		b, e := os.ReadFile(*planPath)
		if e != nil {
			return e
		}
		var plan domain.MaestroPlan
		d := json.NewDecoder(strings.NewReader(string(b)))
		d.DisallowUnknownFields()
		if e = d.Decode(&plan); e != nil {
			return e
		}
		result, e := compose.RelayApply(ctx, cfg, *job, plan)
		if result != "" {
			fmt.Fprintln(out, result)
		}
		return e
	case "run":
		if cfg.BotToken == "" || cfg.WireToken == "" || cfg.ChatID >= 0 || len(cfg.Operators) == 0 || len(cfg.Workspaces) == 0 {
			return errors.New("configure grupo, operadores, workspaces autorizados e tokens Telegram/Wire")
		}
		for _, id := range cfg.Operators {
			if id <= 0 {
				return errors.New("IDs de operadores precisam ser positivos")
			}
		}
		log := slog.New(slog.NewTextHandler(errOut, &slog.HandlerOptions{Level: slog.LevelWarn}))
		return compose.RunRelay(ctx, cfg, log)
	default:
		return fmt.Errorf("comando desconhecido: %s", command)
	}
}

func loadRelayConfig(path string) (domain.RelayConfig, error) {
	var cfg domain.RelayConfig
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err = d.Decode(&cfg); err != nil {
		return cfg, err
	}
	if cfg.StateDir == "" || cfg.CatalogDir == "" || !filepath.IsAbs(cfg.StateDir) || !filepath.IsAbs(cfg.CatalogDir) {
		return cfg, errors.New("stateDir e catalogDir precisam ser caminhos absolutos")
	}
	cfg.BotToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	cfg.WireToken = os.Getenv("MAESTRI_WIRE_TOKEN")
	cfg.LLMKey = os.Getenv("MAESTRI_LLM_KEY")
	if cfg.WireToken == "" {
		b, e := os.ReadFile(filepath.Join(cfg.StateDir, "wire-token"))
		if e == nil {
			cfg.WireToken = strings.TrimSpace(string(b))
		} else if !errors.Is(e, os.ErrNotExist) {
			return cfg, e
		}
	}
	return cfg, nil
}
