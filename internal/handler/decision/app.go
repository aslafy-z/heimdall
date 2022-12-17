package decision

import (
	"strings"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.uber.org/fx"

	"github.com/dadrus/heimdall/internal/cache"
	"github.com/dadrus/heimdall/internal/config"
	accesslogmiddleware "github.com/dadrus/heimdall/internal/fiber/middleware/accesslog"
	cachemiddleware "github.com/dadrus/heimdall/internal/fiber/middleware/cache"
	errormiddleware "github.com/dadrus/heimdall/internal/fiber/middleware/errorhandler"
	loggermiddlerware "github.com/dadrus/heimdall/internal/fiber/middleware/logger"
	tracingmiddleware "github.com/dadrus/heimdall/internal/fiber/middleware/opentelemetry"
	prometheusmiddleware "github.com/dadrus/heimdall/internal/fiber/middleware/prometheus"
	"github.com/dadrus/heimdall/internal/x"
)

type appArgs struct {
	fx.In

	Config     *config.Configuration
	Registerer prometheus.Registerer
	Cache      cache.Cache
	Logger     zerolog.Logger
}

func newApp(args appArgs) *fiber.App {
	service := args.Config.Serve.Decision

	app := fiber.New(fiber.Config{
		AppName:                 "Heimdall Decision Service",
		ReadTimeout:             service.Timeout.Read,
		WriteTimeout:            service.Timeout.Write,
		IdleTimeout:             service.Timeout.Idle,
		DisableStartupMessage:   true,
		EnableTrustedProxyCheck: true,
		TrustedProxies: x.IfThenElseExec(service.TrustedProxies != nil,
			func() []string { return *service.TrustedProxies },
			func() []string { return []string{} }),
		JSONDecoder: json.Unmarshal,
		JSONEncoder: json.Marshal,
	})

	app.Use(recover.New(recover.Config{EnableStackTrace: true}))
	app.Use(tracingmiddleware.New(
		tracingmiddleware.WithTracer(otel.GetTracerProvider().Tracer("github.com/dadrus/heimdall/decision"))))
	app.Use(prometheusmiddleware.New(
		prometheusmiddleware.WithServiceName("decision"),
		prometheusmiddleware.WithRegisterer(args.Registerer),
	))
	app.Use(accesslogmiddleware.New(args.Logger))
	app.Use(loggermiddlerware.New(args.Logger))

	if service.CORS != nil {
		app.Use(cors.New(cors.Config{
			AllowOrigins:     strings.Join(service.CORS.AllowedOrigins, ","),
			AllowMethods:     strings.Join(service.CORS.AllowedMethods, ","),
			AllowHeaders:     strings.Join(service.CORS.AllowedHeaders, ","),
			AllowCredentials: service.CORS.AllowCredentials,
			ExposeHeaders:    strings.Join(service.CORS.ExposedHeaders, ","),
			MaxAge:           int(service.CORS.MaxAge.Seconds()),
		}))
	}

	app.Use(errormiddleware.New(
		errormiddleware.WithVerboseErrors(service.Respond.Verbose),
		errormiddleware.WithPreconditionErrorCode(service.Respond.With.ArgumentError.Code),
		errormiddleware.WithAuthenticationErrorCode(service.Respond.With.AuthenticationError.Code),
		errormiddleware.WithAuthorizationErrorCode(service.Respond.With.AuthorizationError.Code),
		errormiddleware.WithCommunicationErrorCode(service.Respond.With.CommunicationError.Code),
		errormiddleware.WithMethodErrorCode(service.Respond.With.BadMethodError.Code),
		errormiddleware.WithNoRuleErrorCode(service.Respond.With.NoRuleError.Code),
		errormiddleware.WithInternalServerErrorCode(service.Respond.With.InternalError.Code),
	))
	app.Use(cachemiddleware.New(args.Cache))

	return app
}
