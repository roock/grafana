package secret

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/grafana/grafana/pkg/setting"
)

type SaltyValue struct {
	Value  string // the raw value
	Salt   string // random short string (never exposed externally)
	Keeper string // Where the value is encoded (enterprise only, likely empty)
	Addr   string // address within the keeper (enterprise only, likely empty)
}

type SecretKeeper interface {
	Encrypt(ctx context.Context, value SaltyValue) (string, error)
	Decrypt(ctx context.Context, value SaltyValue) (string, error)
}

func ProvideSecretKeeper(cfg *setting.Cfg) (SecretKeeper, error) {
	// TODO... read config and actually use key
	return &simpleKeeper{}, nil
}

var (
	_ SecretKeeper = (*simpleKeeper)(nil)
)

type simpleKeeper struct {
	// key from cfg
}

// Encode implements SecretKeeper.
func (s *simpleKeeper) Encrypt(ctx context.Context, value SaltyValue) (string, error) {
	return base64.StdEncoding.EncodeToString([]byte(value.Salt + value.Value)), nil
}

// Decode implements SecretKeeper.
func (s *simpleKeeper) Decrypt(ctx context.Context, value SaltyValue) (string, error) {
	out, err := base64.StdEncoding.DecodeString(value.Value)
	if err != nil {
		return "", err
	}
	f, ok := strings.CutPrefix(string(out), value.Salt)
	if !ok {
		return "", fmt.Errorf("salt not found in value")
	}
	return f, nil
}
