// Command adminlink prints a signed magic link to the web admin page. It lets a
// web-only operator (no Telegram) open the FAQ/settings/pending-questions editor.
//
//	ASKDESK_MAGIC_LINK_SECRET=... ASKDESK_PUBLIC_URL=https://... make admin-link
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/JasonKyawLab/AskDesk/internal/auth"
	"github.com/JasonKyawLab/AskDesk/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if cfg.MagicLinkSecret == "" {
		fmt.Fprintln(os.Stderr, "error: ASKDESK_MAGIC_LINK_SECRET is required")
		os.Exit(1)
	}
	base := cfg.PublicURL
	if base == "" {
		base = fmt.Sprintf("http://localhost:%d", cfg.HTTPPort)
	}

	link, err := auth.NewSigner(cfg.MagicLinkSecret).MagicLink(base, auth.Claims{
		BusinessID: cfg.BusinessID,
		AdminID:    "cli",
		Channel:    "web",
		ExpiresAt:  time.Now().Add(10 * time.Minute).Unix(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(link)
}
