package app

import "testing"

func TestServerAddress(t *testing.T) {
	for _, tc := range []struct{ host, want string }{
		{"", "127.0.0.1:8081"},
		{"0.0.0.0", "0.0.0.0:8081"},
		{"::1", "[::1]:8081"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			t.Setenv("SERVER_HOST", tc.host)
			if got := serverAddress("8081"); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
