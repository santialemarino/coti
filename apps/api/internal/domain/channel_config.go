package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/mail"
	"strings"
)

// maxChannelSecretLength bounds a credential before it is sealed. Nothing revalidates a stored
// config, which is what lets the bound sit on the plaintext: the envelope is longer than what it
// wraps.
const maxChannelSecretLength = 4096

// maxChannelFieldLength bounds every readable field of a config.
const maxChannelFieldLength = 255

// ChannelConfig is a channel's stored settings, one shape per channel type.
// ParseChannelConfig is the only way to get one, and is also what validates it.
type ChannelConfig interface {
	// MapSecrets rewrites each non-empty credential field through fn, in place.
	MapSecrets(fn func(string) (string, error)) error
}

// WhatsAppChannelConfig is what a WHATSAPP channel stores: the provider's own references for the
// number, and the credentials to call it with. The number itself is channel.identifier and is not
// repeated here — channel uniqueness rests on that column.
type WhatsAppChannelConfig struct {
	PhoneNumberID      string `json:"phone_number_id"`
	BusinessAccountID  string `json:"business_account_id,omitempty"`
	AccessToken        string `json:"access_token"`
	WebhookVerifyToken string `json:"webhook_verify_token,omitempty"`
}

// EmailChannelConfig is what an EMAIL channel stores: the mailbox orders arrive at, and the
// credentials that send from it.
type EmailChannelConfig struct {
	Mailbox      string `json:"mailbox"`
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	// SMTPStartTLS is declared rather than negotiated, so a server that stops advertising it
	// fails the send instead of quietly downgrading to plaintext.
	SMTPStartTLS bool `json:"smtp_starttls"`
}

// ParseChannelConfig decodes and validates the settings a channel of this type accepts. An absent
// or empty object is no configuration at all, which every type accepts: a channel exists before
// its credentials do. An unknown field is refused rather than stored — a free-form object where
// credentials live is a dump nobody can reason about later.
func ParseChannelConfig(channelType ChannelType, raw []byte) (ChannelConfig, error) {
	if isEmptyChannelConfig(raw) {
		return nil, nil
	}

	switch channelType {
	case ChannelTypeWhatsApp:
		var config WhatsAppChannelConfig
		if err := decodeChannelConfig(raw, &config); err != nil {
			return nil, err
		}
		if err := config.validate(); err != nil {
			return nil, err
		}
		return &config, nil
	case ChannelTypeEmail:
		var config EmailChannelConfig
		if err := decodeChannelConfig(raw, &config); err != nil {
			return nil, err
		}
		if err := config.validate(); err != nil {
			return nil, err
		}
		return &config, nil
	case ChannelTypeWebApp, ChannelTypeManualEntry:
		return nil, channelConfigError("a %s channel takes no configuration", channelType)
	default:
		return nil, channelConfigError("unknown channel type %q", channelType)
	}
}

// MapSecrets rewrites the WhatsApp credentials through fn.
func (c *WhatsAppChannelConfig) MapSecrets(fn func(string) (string, error)) error {
	return mapChannelSecrets(fn, &c.AccessToken, &c.WebhookVerifyToken)
}

func (c *WhatsAppChannelConfig) validate() error {
	if err := requireChannelField("phone_number_id", c.PhoneNumberID, maxChannelFieldLength); err != nil {
		return err
	}
	if err := boundChannelField("business_account_id", c.BusinessAccountID, maxChannelFieldLength); err != nil {
		return err
	}
	if err := requireChannelField("access_token", c.AccessToken, maxChannelSecretLength); err != nil {
		return err
	}
	return boundChannelField("webhook_verify_token", c.WebhookVerifyToken, maxChannelSecretLength)
}

// MapSecrets rewrites the sending password through fn.
func (c *EmailChannelConfig) MapSecrets(fn func(string) (string, error)) error {
	return mapChannelSecrets(fn, &c.SMTPPassword)
}

func (c *EmailChannelConfig) validate() error {
	if err := requireChannelField("mailbox", c.Mailbox, maxChannelFieldLength); err != nil {
		return err
	}
	if _, err := mail.ParseAddress(c.Mailbox); err != nil {
		return channelConfigError("mailbox must be an email address")
	}
	if err := requireChannelField("smtp_host", c.SMTPHost, maxChannelFieldLength); err != nil {
		return err
	}
	if c.SMTPPort < 1 || c.SMTPPort > 65535 {
		return channelConfigError("smtp_port must be between 1 and 65535")
	}
	if err := requireChannelField("smtp_username", c.SMTPUsername, maxChannelFieldLength); err != nil {
		return err
	}
	return requireChannelField("smtp_password", c.SMTPPassword, maxChannelSecretLength)
}

// isEmptyChannelConfig reports whether raw carries no settings — absent, null, or an object with
// no keys. All three say the same thing: this channel is not configured yet.
func isEmptyChannelConfig(raw []byte) bool {
	if len(bytes.TrimSpace(raw)) == 0 {
		return true
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return false
	}
	return len(fields) == 0
}

func decodeChannelConfig(raw []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return channelConfigError("%s", strings.TrimPrefix(err.Error(), "json: "))
	}
	return nil
}

// mapChannelSecrets maps every credential before writing any of them: one that failed halfway
// would leave a config holding one sealed credential beside one in the clear.
func mapChannelSecrets(fn func(string) (string, error), fields ...*string) error {
	mapped := make([]string, len(fields))
	for i, field := range fields {
		if *field == "" {
			continue
		}
		value, err := fn(*field)
		if err != nil {
			return err
		}
		mapped[i] = value
	}
	for i, field := range fields {
		if *field != "" {
			*field = mapped[i]
		}
	}
	return nil
}

func requireChannelField(name, value string, maxLength int) error {
	if strings.TrimSpace(value) == "" {
		return channelConfigError("%s is required", name)
	}
	return boundChannelField(name, value, maxLength)
}

func boundChannelField(name, value string, maxLength int) error {
	if len(value) > maxLength {
		return channelConfigError("%s must be at most %d bytes", name, maxLength)
	}
	return nil
}

func channelConfigError(format string, args ...any) error {
	return WithCode(CodeChannelConfigShape, fmt.Errorf("%w: channel config: %s",
		ErrInvalidInput, fmt.Sprintf(format, args...)))
}
