package f5

import (
	"context"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/term"
	"github.com/tj/go-terminput"
)

var (
	// extension of top langauges
	supportedExtensionMap = map[string]bool{}
	supportedExtensions   = []string{
		".py", ".js", ".java", ".ts", ".go",
		".cpp", ".rb", ".php", ".cs", ".c",
	}
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
	separator   = "------------------------------------------------------------------"
)

func init() {
	for _, s := range supportedExtensions {
		supportedExtensionMap[s] = true
	}
}

func (r *Run) printf(color string, format string, a ...any) {
	f := color + format + colorReset
	r.logger.Printf(f, a...)
}

func (r *Run) usagef(color string, format string, a ...any) {
	f := color + format + colorReset
	r.usage.Printf(f, a...)
}

type Run struct {
	args    []string
	process *os.Process
	watcher *fsnotify.Watcher
	term    *term.Term

	restart chan bool
	logger  *log.Logger
	usage   *log.Logger
}

func New(args ...string) (*Run, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	t, err := term.Open("/dev/tty")
	if err != nil {
		return nil, err
	}

	fn := filepath.Base(args[0])
	prefix := fmt.Sprintf("%s[Press F5 to refresh %q] %s", colorGreen, fn, colorReset)
	logger := log.New(os.Stderr, prefix, log.LstdFlags)
	usage := log.New(os.Stderr, prefix, 0)
	r := Run{
		args:    args,
		restart: make(chan bool, 100),
		watcher: watcher,
		term:    t,
		logger:  logger,
		usage:   usage,
	}
	return &r, nil
}

func (r *Run) kill() {
	if r.process != nil {
		pid := r.process.Pid
		err := syscall.Kill(-pid, syscall.SIGINT)
		if err != nil && !strings.Contains(err.Error(), "no such process") {
			r.printf(colorRed, "Process %d: cannot interrupt: %v", pid, err)
			r.printf(colorPurple, "Process %d: sending sigkill", pid)
			err := syscall.Kill(-pid, syscall.SIGKILL)
			if err != nil {
				r.printf(colorRed, "Process %d: cannot be killed: %v", pid, err)
			}
		}
		r.process = nil
	}
}

func (r *Run) Close() {
	r.term.Restore()
	r.watcher.Close()
	r.kill()
}

func (r *Run) Restart(ctx context.Context) {
	r.kill()
	cmd := exec.Command(r.args[0], r.args[1:]...)
	// set process group, so we can kill all of the spawned processes.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		r.printf(colorRed, "Cannot run command: %v", err)
		return
	}
	r.process = cmd.Process
	fmt.Printf("%s%s\n", colorGreen, separator)
	r.printf(colorWhite, "Process %d started for command: %s%s", cmd.Process.Pid, colorCyan, cmd)
	fmt.Printf("%s%s%s\n", colorGreen, separator, colorReset)

	go cmd.Wait()

}

func (r *Run) Start(ctx context.Context) error {
	fmt.Printf("%s%s\n", colorGreen, separator)
	r.usagef(colorWhite, "To restart the running program, press F5 or SPACE or Ctrl-R, or just make file changes.")
	go func() {
		for {
			select {
			case <-r.restart:
				r.Restart(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()

	defer func() {
		r.restart <- true
	}()

	return r.watch(ctx)
}

func (r *Run) ListenForKeys(ctx context.Context) {
	r.term.SetCbreak()
	defer r.term.Restore()
	for {
		if ctx.Err() != nil {
			return
		}
		e, _ := terminput.Read(r.term)
		// log.Printf("got: %s", e.String())
		switch e.String() {
		case "DC2":
			fallthrough
		case " ":
			fallthrough
		case "F5":
			r.Restart(ctx)
		}
	}
}

func (r *Run) watch(ctx context.Context) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	dirs := []string{}
	filepath.WalkDir(wd, func(s string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		// skip hidden directories with . as prefix
		if strings.HasPrefix(filepath.Base(s), ".") {
			return filepath.SkipDir
		}
		// check if the directory has go code.
		files, err := ioutil.ReadDir(s)
		if err != nil {
			return err
		}
		for _, f := range files {
			if supportedExtensionMap[filepath.Ext(f.Name())] {
				dirs = append(dirs, s)
				return nil
			}
		}
		return nil
	})
	r.usagef(colorWhite, "The following directories are being monitored")
	for i, d := range dirs {
		r.usagef(colorWhite, "%3d. %s", i+1, d)
		r.watcher.Add(d)
	}

	// watch until error or cancelled.
	go func() {
		defer r.watcher.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-r.watcher.Events:
				if !ok {
					r.printf(colorRed, "Unknown event, halting.")
					return
				}
				if event.Op&fsnotify.Write != fsnotify.Write {
					continue
				}
				if !supportedExtensionMap[filepath.Ext(event.Name)] {
					continue
				}
				r.printf(colorGreen, "Modified file: %s", event.Name)
				r.restart <- true
			case err, ok := <-r.watcher.Errors:
				if !ok {
					r.printf(colorRed, "Unknown error, halting.")
					return
				}
				r.printf(colorRed, "Error:", err)
			}
		}
	}()

	return nil
}
