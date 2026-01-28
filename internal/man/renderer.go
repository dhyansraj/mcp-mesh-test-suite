package man

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
)

// Renderer handles terminal output for man pages.
type Renderer struct {
	out io.Writer
}

// NewRenderer creates a new renderer with the given output writer.
func NewRenderer(out io.Writer) *Renderer {
	if out == nil {
		out = os.Stdout
	}
	return &Renderer{out: out}
}

// RenderPage renders a man page to the terminal with markdown formatting.
func (r *Renderer) RenderPage(page *ManPage) error {
	content, err := page.GetContent()
	if err != nil {
		return fmt.Errorf("failed to read content: %w", err)
	}

	// Use glamour for terminal markdown rendering
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		// Fallback to plain text if glamour fails
		fmt.Fprintln(r.out)
		fmt.Fprintln(r.out, content)
		fmt.Fprintln(r.out)
		return nil
	}

	rendered, err := renderer.Render(content)
	if err != nil {
		// Fallback to plain text
		fmt.Fprintln(r.out)
		fmt.Fprintln(r.out, content)
		fmt.Fprintln(r.out)
		return nil
	}

	fmt.Fprint(r.out, rendered)
	return nil
}

// RenderList renders the list of available man pages.
func (r *Renderer) RenderList() {
	fmt.Fprintln(r.out)
	fmt.Fprintln(r.out, "\033[1;36mAvailable Topics\033[0m")
	fmt.Fprintln(r.out, "\033[36m"+strings.Repeat("─", 72)+"\033[0m")
	fmt.Fprintln(r.out)

	for _, page := range ListPages() {
		// Topic name in yellow/bold
		fmt.Fprintf(r.out, "  \033[1;33m%s\033[0m", page.Name)

		// Aliases in dim
		if len(page.Aliases) > 0 {
			fmt.Fprintf(r.out, " \033[2m(%s)\033[0m", strings.Join(page.Aliases, ", "))
		}
		fmt.Fprintln(r.out)

		// Description indented
		fmt.Fprintf(r.out, "    %s\n", page.Description)
		fmt.Fprintln(r.out)
	}
}

// RenderNotFound renders a "topic not found" message.
func (r *Renderer) RenderNotFound(topic string) {
	fmt.Fprintln(r.out)
	fmt.Fprintf(r.out, "\033[1;31mTopic not found: %s\033[0m\n", topic)
	fmt.Fprintln(r.out)
	fmt.Fprintln(r.out, "Use 'tsuite man --list' to see available topics.")
	fmt.Fprintln(r.out)
}
