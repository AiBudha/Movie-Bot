package index

import (
	"errors"
	"testing"
)

func TestParseMtProtoFloodwait(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		expectSeconds int64
		expectOk      bool
		expectErr     bool
	}{
		{
			name:          "standard gotgbot/mtproto format with 'Please wait'",
			err:           errors.New("sending ChannelsGetMessages: [FLOOD_WAIT_X] Please wait 31 seconds before repeating the action. (method: ChannelsGetMessages) (code 420)"),
			expectSeconds: 31,
			expectOk:      true,
			expectErr:     false,
		},
		{
			name:          "legacy format with 'wait of'",
			err:           errors.New("sending ChannelsGetMessages: [FLOOD_WAIT_X] wait of 5 seconds (method: ChannelsGetMessages) (code 420)"),
			expectSeconds: 5,
			expectOk:      true,
			expectErr:     false,
		},
		{
			name:          "simple format",
			err:           errors.New("FLOOD_WAIT_X: Please wait 120 seconds"),
			expectSeconds: 120,
			expectOk:      true,
			expectErr:     false,
		},
		{
			name:          "no flood wait error",
			err:           errors.New("some other error: not found"),
			expectSeconds: 0,
			expectOk:      false,
			expectErr:     false,
		},
		{
			name:          "flood wait error but no seconds parsed",
			err:           errors.New("[FLOOD_WAIT_X] something went wrong"),
			expectSeconds: 0,
			expectOk:      true,
			expectErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seconds, ok, err := ParseMtProtoFloodwait(tt.err)
			if ok != tt.expectOk {
				t.Errorf("expected ok=%v, got %v", tt.expectOk, ok)
			}
			if seconds != tt.expectSeconds {
				t.Errorf("expected seconds=%d, got %d", tt.expectSeconds, seconds)
			}
			if (err != nil) != tt.expectErr {
				t.Errorf("expected err presence=%v, got %v", tt.expectErr, err)
			}
		})
	}
}

func TestTDLibChannelIDToPlain(t *testing.T) {
	tests := []struct {
		input    int64
		expected int64
	}{
		{-1001234567890, 1234567890},
		{-1002000000000, 2000000000},
	}

	for _, tt := range tests {
		result := TDLibChannelIDToPlain(tt.input)
		if result != tt.expected {
			t.Errorf("TDLibChannelIDToPlain(%d) = %d; expected %d", tt.input, result, tt.expected)
		}
	}
}
