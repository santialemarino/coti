package services

import (
	"bytes"
	"fmt"
	"html/template"
	"regexp"
	"strings"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// defaultBrandColor is what the header falls back to when the account has not set one.
const defaultBrandColor = "#1F2937"

// brandColorPattern gates what reaches the stylesheet: html/template blanks any value it
// cannot prove is a CSS token, so an unchecked colour renders as no colour at all.
var brandColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{3}([0-9A-Fa-f]{3}([0-9A-Fa-f]{2})?)?$`)

// emailView is what the layout renders: the account's brand plus the caller's content. Every
// user-facing string reaches it from mail_copy.go, so the layout itself carries no copy.
type emailView struct {
	AccountName    string
	LogoURL        string
	BrandColor     template.CSS
	Heading        string
	Paragraphs     []string
	ActionLabel    string
	ActionURL      string
	ActionFallback string
}

var emailHTMLTemplate = template.Must(template.New("email").Parse(
	`<!doctype html>
<html lang="es">
<body style="margin:0;padding:0;background:#F3F4F6;font-family:Arial,Helvetica,sans-serif;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="padding:24px 0;">
<tr><td align="center">
<table role="presentation" width="560" cellpadding="0" cellspacing="0" style="background:#FFFFFF;border-radius:8px;overflow:hidden;">
<tr><td style="background:{{.BrandColor}};padding:20px 28px;">
{{if .LogoURL}}<img src="{{.LogoURL}}" alt="{{.AccountName}}" height="32" style="display:block;border:0;">
{{else}}<span style="color:#FFFFFF;font-size:18px;font-weight:bold;">{{.AccountName}}</span>{{end}}
</td></tr>
<tr><td style="padding:28px;">
<h1 style="margin:0 0 16px;font-size:20px;color:#111827;">{{.Heading}}</h1>
{{range .Paragraphs}}<p style="margin:0 0 14px;font-size:15px;line-height:1.5;color:#374151;">{{.}}</p>
{{end}}
{{if .ActionURL}}<p style="margin:22px 0;"><a href="{{.ActionURL}}" style="display:inline-block;background:{{.BrandColor}};color:#FFFFFF;text-decoration:none;padding:12px 22px;border-radius:6px;font-size:15px;">{{.ActionLabel}}</a></p>
<p style="margin:0;font-size:13px;color:#6B7280;">{{.ActionFallback}}<br>{{.ActionURL}}</p>
{{end}}
</td></tr>
<tr><td style="padding:18px 28px;background:#F9FAFB;font-size:12px;color:#6B7280;">{{.AccountName}}</td></tr>
</table>
</td></tr>
</table>
</body>
</html>
`))

// renderEmail turns an outbound message into the transport's payload, wrapped in the
// account's brand.
func renderEmail(account domain.Account, out OutboundMail) (domain.EmailMessage, error) {
	view := emailView{
		AccountName:    account.Name,
		BrandColor:     template.CSS(defaultBrandColor),
		Heading:        out.Heading,
		Paragraphs:     out.Paragraphs,
		ActionLabel:    out.ActionLabel,
		ActionURL:      out.ActionURL,
		ActionFallback: mailActionFallback,
	}
	if account.BrandLogoURL != nil {
		view.LogoURL = *account.BrandLogoURL
	}
	if account.BrandColor != nil && brandColorPattern.MatchString(*account.BrandColor) {
		view.BrandColor = template.CSS(*account.BrandColor)
	}

	var html bytes.Buffer
	if err := emailHTMLTemplate.Execute(&html, view); err != nil {
		return domain.EmailMessage{}, fmt.Errorf("render email: %w", err)
	}

	return domain.EmailMessage{
		To:       out.To,
		ToName:   out.ToName,
		Subject:  out.Subject,
		TextBody: renderEmailText(view),
		HTMLBody: html.String(),
	}, nil
}

// renderEmailText is the plain-text alternative, which is also what the console transport
// writes to the log.
func renderEmailText(view emailView) string {
	var b strings.Builder
	b.WriteString(view.Heading)
	b.WriteString("\n\n")
	for _, p := range view.Paragraphs {
		b.WriteString(p)
		b.WriteString("\n\n")
	}
	if view.ActionURL != "" {
		b.WriteString(view.ActionLabel)
		b.WriteString(": ")
		b.WriteString(view.ActionURL)
		b.WriteString("\n\n")
	}
	b.WriteString("— ")
	b.WriteString(view.AccountName)
	return b.String()
}
