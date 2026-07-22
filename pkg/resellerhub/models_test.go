package resellerhub

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResellerBeforeCreateGeneratesCodeWhenOmitted(t *testing.T) {
	reseller := &Reseller{}

	err := reseller.BeforeCreate(nil)

	require.NoError(t, err)
	assert.Regexp(t, regexp.MustCompile(`^res_[0-9a-f]{32}$`), reseller.Code)
}

func TestResellerBeforeCreatePreservesExplicitCode(t *testing.T) {
	reseller := &Reseller{Code: "partner-apac"}

	err := reseller.BeforeCreate(nil)

	require.NoError(t, err)
	assert.Equal(t, "partner-apac", reseller.Code)
}
