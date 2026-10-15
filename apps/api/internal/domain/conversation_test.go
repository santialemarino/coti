package domain

import (
	"errors"
	"slices"
	"testing"
)

func TestInboundSenderDestinations(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		channelType ChannelType
		sender      string
		want        []string
		wantRefused bool
	}{
		{name: "argentine mobile with the 9", channelType: ChannelTypeWhatsApp,
			sender: "5491112345678", want: []string{"+5491112345678", "+541112345678"}},
		{name: "argentine mobile without the 9", channelType: ChannelTypeWhatsApp,
			sender: "541112345678", want: []string{"+541112345678", "+5491112345678"}},
		{name: "another country", channelType: ChannelTypeWhatsApp,
			sender: "59899123456", want: []string{"+59899123456"}},
		{name: "a leading plus is tolerated", channelType: ChannelTypeWhatsApp,
			sender: " +59899123456 ", want: []string{"+59899123456"}},
		{name: "letters", channelType: ChannelTypeWhatsApp, sender: "54911abc", wantRefused: true},
		{name: "empty", channelType: ChannelTypeWhatsApp, sender: " ", wantRefused: true},
		{name: "email is not routed yet", channelType: ChannelTypeEmail,
			sender: "obra@cliente.test", wantRefused: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := InboundSenderDestinations(test.channelType, test.sender)
			if test.wantRefused {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("InboundSenderDestinations() = %v, %v; want ErrInvalidInput", got, err)
				}
				return
			}
			if err != nil || !slices.Equal(got, test.want) {
				t.Errorf("InboundSenderDestinations() = %v, %v; want %v", got, err, test.want)
			}
		})
	}
}
