package extractors

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dadrus/heimdall/internal/test"
)

func TestCompositeExtractCookieValueWithoutPrefix(t *testing.T) {
	t.Parallel()

	// GIVEN
	formParamName := "test_param"
	cookieName := "Test-Cookie"
	actualValue := "foo"
	mock := &test.MockAuthDataSource{}
	mock.On("Cookie", cookieName).Return(actualValue)
	mock.On("Form", formParamName).Return("")

	strategy := CompositeExtractStrategy{
		FormParameterExtractStrategy{Name: formParamName},
		CookieValueExtractStrategy{Name: cookieName},
	}

	// WHEN
	val, err := strategy.GetAuthData(mock)

	// THEN
	assert.NoError(t, err)
	assert.Equal(t, actualValue, val)
	mock.AssertExpectations(t)
}

func TestCompositeExtractHeaderValueWithPrefix(t *testing.T) {
	t.Parallel()

	// GIVEN
	headerName := "Test-Header"
	queryParamName := "test_param"
	valuePrefix := "bar:"
	actualValue := "foo"
	mock := &test.MockAuthDataSource{}
	mock.On("Header", headerName).Return(valuePrefix + " " + actualValue)
	mock.On("Query", queryParamName).Return("")

	strategy := CompositeExtractStrategy{
		QueryParameterExtractStrategy{Name: queryParamName},
		HeaderValueExtractStrategy{Name: headerName, Prefix: valuePrefix},
	}

	// WHEN
	val, err := strategy.GetAuthData(mock)

	// THEN
	assert.NoError(t, err)
	assert.Equal(t, actualValue, val)
	mock.AssertExpectations(t)
}

func TestCompositeExtractStrategyOrder(t *testing.T) {
	t.Parallel()

	// GIVEN
	headerName := "Test-Header"
	queryParamName := "test_param"
	valuePrefix := "bar:"
	actualValue := "foo"
	mock := &test.MockAuthDataSource{}
	mock.On("Header", headerName).Return(valuePrefix + " " + actualValue)

	strategy := CompositeExtractStrategy{
		HeaderValueExtractStrategy{Name: headerName, Prefix: valuePrefix},
		QueryParameterExtractStrategy{Name: queryParamName},
	}

	// WHEN
	val, err := strategy.GetAuthData(mock)

	// THEN
	assert.NoError(t, err)
	assert.Equal(t, actualValue, val)
	mock.AssertExpectations(t)
}
