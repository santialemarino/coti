package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const validWhatsAppConfig = `{"phone_number_id":"1234567890","access_token":"EAAG-token"}`

const validEmailConfig = `{"smtp_host":"smtp.corralon.test","smtp_port":587,` +
	`"smtp_username":"pedidos","smtp_password":"s3cret","smtp_starttls":true}`

func TestParseChannelConfig_AbsentConfigIsValidForEveryType(t *testing.T) {
	t.Parallel()

	for _, channelType := range channelTypes {
		for _, raw := range []string{"", "  ", "null", "{}", "{ }"} {
			config, err := ParseChannelConfig(channelType, []byte(raw))
			if err != nil {
				t.Errorf("ParseChannelConfig(%s, %q) = %v, want no error", channelType, raw, err)
			}
			if config != nil {
				t.Errorf("ParseChannelConfig(%s, %q) = %#v, want nil", channelType, raw, config)
			}
		}
	}
}

func TestParseChannelConfig_ShapePerType(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		channelType ChannelType
		raw         string
		wantErr     string
	}{
		{name: "whatsapp minimal", channelType: ChannelTypeWhatsApp, raw: validWhatsAppConfig},
		{name: "whatsapp full", channelType: ChannelTypeWhatsApp, raw: `{"phone_number_id":"1",
			"business_account_id":"2","access_token":"t","webhook_verify_token":"v"}`},
		{name: "whatsapp without a number reference", channelType: ChannelTypeWhatsApp,
			raw: `{"access_token":"t"}`, wantErr: "phone_number_id is required"},
		{name: "whatsapp without credentials", channelType: ChannelTypeWhatsApp,
			raw: `{"phone_number_id":"1"}`, wantErr: "access_token is required"},
		{name: "whatsapp with a blank credential", channelType: ChannelTypeWhatsApp,
			raw: `{"phone_number_id":"1","access_token":"   "}`, wantErr: "access_token is required"},
		{name: "whatsapp with an unknown field", channelType: ChannelTypeWhatsApp,
			raw:     `{"phone_number_id":"1","access_token":"t","secret":"x"}`,
			wantErr: `unknown field "secret"`},
		{name: "whatsapp with the identifier inside", channelType: ChannelTypeWhatsApp,
			raw:     `{"phone_number_id":"1","access_token":"t","identifier":"+5491100000000"}`,
			wantErr: `unknown field "identifier"`},
		{name: "whatsapp with an email shape", channelType: ChannelTypeWhatsApp,
			raw: validEmailConfig, wantErr: `unknown field "smtp_host"`},
		{name: "email minimal", channelType: ChannelTypeEmail, raw: validEmailConfig},
		{name: "email with the mailbox inside", channelType: ChannelTypeEmail,
			raw: `{"mailbox":"a@b.test","smtp_host":"h","smtp_port":587,"smtp_username":"u",
				"smtp_password":"p"}`,
			wantErr: `unknown field "mailbox"`},
		{name: "email without a host", channelType: ChannelTypeEmail,
			raw:     `{"smtp_port":587,"smtp_username":"u","smtp_password":"p"}`,
			wantErr: "smtp_host is required"},
		{name: "email without a port", channelType: ChannelTypeEmail,
			raw:     `{"smtp_host":"h","smtp_username":"u","smtp_password":"p"}`,
			wantErr: "smtp_port must be between 1 and 65535"},
		{name: "email with an out-of-range port", channelType: ChannelTypeEmail,
			raw:     `{"smtp_host":"h","smtp_port":65536,"smtp_username":"u","smtp_password":"p"}`,
			wantErr: "smtp_port must be between 1 and 65535"},
		{name: "email with a port that is not a number", channelType: ChannelTypeEmail,
			raw:     `{"smtp_host":"h","smtp_port":"587","smtp_username":"u","smtp_password":"p"}`,
			wantErr: "smtp_port must be a int, got string"},
		{name: "whatsapp with a credential that is not a string",
			channelType: ChannelTypeWhatsApp,
			raw:         `{"phone_number_id":"1","access_token":["t"]}`,
			wantErr:     "access_token must be a string, got array"},
		{name: "email without a password", channelType: ChannelTypeEmail,
			raw:     `{"smtp_host":"h","smtp_port":587,"smtp_username":"u"}`,
			wantErr: "smtp_password is required"},
		{name: "email with a whatsapp shape", channelType: ChannelTypeEmail,
			raw: validWhatsAppConfig, wantErr: `unknown field "phone_number_id"`},
		{name: "webapp with a config", channelType: ChannelTypeWebApp,
			raw: `{"anything":"x"}`, wantErr: "a WEBAPP channel takes no configuration"},
		{name: "manual entry with a config", channelType: ChannelTypeManualEntry,
			raw: validWhatsAppConfig, wantErr: "a MANUAL_ENTRY channel takes no configuration"},
		{name: "unknown type", channelType: ChannelType("TELEGRAM"),
			raw: `{"a":"b"}`, wantErr: `unknown channel type "TELEGRAM"`},
		{name: "not an object", channelType: ChannelTypeWhatsApp,
			raw: `"a string"`, wantErr: "must be an object of the fields this channel type accepts"},
		{name: "an array", channelType: ChannelTypeWhatsApp,
			raw: `["a"]`, wantErr: "must be an object of the fields this channel type accepts"},
		{name: "a number", channelType: ChannelTypeEmail,
			raw: `7`, wantErr: "must be an object of the fields this channel type accepts"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := ParseChannelConfig(test.channelType, []byte(test.raw))
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("ParseChannelConfig() = %v, want no error", err)
				}
				if config == nil {
					t.Fatal("ParseChannelConfig() = nil, want a config")
				}
				return
			}
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("ParseChannelConfig() = %v, want %v", err, ErrInvalidInput)
			}
			if CodeOf(err) != CodeChannelConfigShape {
				t.Errorf("CodeOf() = %v, want %v", CodeOf(err), CodeChannelConfigShape)
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("ParseChannelConfig() = %q, want it to mention %q", err, test.wantErr)
			}
			if config != nil {
				t.Errorf("ParseChannelConfig() = %#v, want nil alongside an error", config)
			}
		})
	}
}

func TestParseChannelConfig_BoundsEveryField(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		channelType ChannelType
		raw         string
		wantErr     string
	}{
		{name: "phone number reference", channelType: ChannelTypeWhatsApp,
			raw:     `{"phone_number_id":"` + strings.Repeat("1", 256) + `","access_token":"t"}`,
			wantErr: "phone_number_id must be at most 255 bytes"},
		{name: "business account", channelType: ChannelTypeWhatsApp,
			raw: `{"phone_number_id":"1","business_account_id":"` + strings.Repeat("2", 256) +
				`","access_token":"t"}`,
			wantErr: "business_account_id must be at most 255 bytes"},
		{name: "access token", channelType: ChannelTypeWhatsApp,
			raw:     `{"phone_number_id":"1","access_token":"` + strings.Repeat("t", 4097) + `"}`,
			wantErr: "access_token must be at most 4096 bytes"},
		{name: "webhook token", channelType: ChannelTypeWhatsApp,
			raw: `{"phone_number_id":"1","access_token":"t","webhook_verify_token":"` +
				strings.Repeat("v", 4097) + `"}`,
			wantErr: "webhook_verify_token must be at most 4096 bytes"},
		{name: "smtp host", channelType: ChannelTypeEmail,
			raw: `{"smtp_host":"` + strings.Repeat("h", 256) +
				`","smtp_port":587,"smtp_username":"u","smtp_password":"p"}`,
			wantErr: "smtp_host must be at most 255 bytes"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseChannelConfig(test.channelType, []byte(test.raw))
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("ParseChannelConfig() = %v, want %v", err, ErrInvalidInput)
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("ParseChannelConfig() = %q, want it to mention %q", err, test.wantErr)
			}
		})
	}
}

func TestChannelConfig_MapSecretsCoversEveryCredential(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		channelType ChannelType
		raw         string
		wantSealed  []string
		wantClear   []string
	}{
		{
			name: "whatsapp", channelType: ChannelTypeWhatsApp,
			raw: `{"phone_number_id":"1234567890","business_account_id":"9876",
				"access_token":"EAAG-token","webhook_verify_token":"hook-token"}`,
			wantSealed: []string{"access_token", "webhook_verify_token"},
			wantClear:  []string{"phone_number_id", "business_account_id"},
		},
		{
			name: "email", channelType: ChannelTypeEmail, raw: validEmailConfig,
			wantSealed: []string{"smtp_password"},
			wantClear:  []string{"smtp_host", "smtp_username"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := ParseChannelConfig(test.channelType, []byte(test.raw))
			if err != nil {
				t.Fatalf("ParseChannelConfig() = %v, want no error", err)
			}
			before := fieldsOf(t, config)

			if err := config.MapSecrets(func(v string) (string, error) {
				return "sealed:" + v, nil
			}); err != nil {
				t.Fatalf("MapSecrets() = %v, want no error", err)
			}
			after := fieldsOf(t, config)

			for _, field := range test.wantSealed {
				if after[field] != "sealed:"+before[field] {
					t.Errorf("%s = %q, want it sealed from %q", field, after[field], before[field])
				}
			}
			for _, field := range test.wantClear {
				if after[field] != before[field] {
					t.Errorf("%s = %q, want it left at %q", field, after[field], before[field])
				}
			}
			if len(test.wantSealed)+len(test.wantClear) != len(after) {
				t.Errorf("config has %d string fields, and the test names %d: a new field is "+
					"unclassified, so nothing says whether it is a credential",
					len(after), len(test.wantSealed)+len(test.wantClear))
			}
		})
	}
}

func TestChannelConfig_MapSecretsSkipsTheAbsentOnesAndReportsFailure(t *testing.T) {
	t.Parallel()
	config, err := ParseChannelConfig(ChannelTypeWhatsApp, []byte(validWhatsAppConfig))
	if err != nil {
		t.Fatalf("ParseChannelConfig() = %v, want no error", err)
	}

	calls := 0
	if err := config.MapSecrets(func(v string) (string, error) {
		calls++
		return v, nil
	}); err != nil {
		t.Fatalf("MapSecrets() = %v, want no error", err)
	}
	if calls != 1 {
		t.Errorf("MapSecrets() called fn %d times, want 1: the webhook token is absent", calls)
	}

	wantErr := errors.New("sealing failed")
	if err := config.MapSecrets(func(string) (string, error) {
		return "", wantErr
	}); !errors.Is(err, wantErr) {
		t.Errorf("MapSecrets() = %v, want %v", err, wantErr)
	}
}

func TestChannelConfig_MapSecretsFailingOnTheSecondWritesNeither(t *testing.T) {
	t.Parallel()
	config, err := ParseChannelConfig(ChannelTypeWhatsApp, []byte(
		`{"phone_number_id":"1","access_token":"first","webhook_verify_token":"second"}`))
	if err != nil {
		t.Fatalf("ParseChannelConfig() = %v, want no error", err)
	}

	wantErr := errors.New("sealing failed")
	mapErr := config.MapSecrets(func(v string) (string, error) {
		if v == "second" {
			return "", wantErr
		}
		return "sealed:" + v, nil
	})
	if !errors.Is(mapErr, wantErr) {
		t.Fatalf("MapSecrets() = %v, want %v", mapErr, wantErr)
	}

	whatsApp, ok := config.(*WhatsAppChannelConfig)
	if !ok {
		t.Fatalf("config is %T, want *WhatsAppChannelConfig", config)
	}
	if whatsApp.AccessToken != "first" || whatsApp.WebhookVerifyToken != "second" {
		t.Errorf("credentials = %q / %q, want them both untouched: a half-written map leaves one "+
			"sealed beside one in the clear", whatsApp.AccessToken, whatsApp.WebhookVerifyToken)
	}
}

// fieldsOf reads every string field of a config back out through its JSON shape, so a field added
// to a shape without being classified as a credential shows up in the count.
func fieldsOf(t *testing.T, config ChannelConfig) map[string]string {
	t.Helper()
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	var all map[string]any
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	fields := make(map[string]string)
	for name, value := range all {
		if text, isText := value.(string); isText {
			fields[name] = text
		}
	}
	return fields
}

// The whole-value message names no Go type, and no message ever echoes a credential's contents: a
// string mismatch is described as "string", never quoted.
func TestParseChannelConfig_ErrorsNameNoInternalTypeAndNoSecret(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`"EAAG-super-secret"`,
		`{"phone_number_id":"1","access_token":["EAAG-super-secret"]}`,
		`{"phone_number_id":"1","access_token":{"v":"EAAG-super-secret"}}`,
	} {
		_, err := ParseChannelConfig(ChannelTypeWhatsApp, []byte(raw))
		if err == nil {
			t.Fatalf("ParseChannelConfig(%s) = nil, want an error", raw)
		}
		if strings.Contains(err.Error(), "EAAG-super-secret") {
			t.Errorf("ParseChannelConfig(%s) = %q, want the credential left out", raw, err)
		}
		for _, leak := range []string{"domain.", "WhatsAppChannelConfig", "EmailChannelConfig"} {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("ParseChannelConfig(%s) = %q, want %q left out", raw, err, leak)
			}
		}
	}
}
