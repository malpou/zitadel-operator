// Command zitadel-operator reconciles zitadel-operator.io CRDs against one
// Zitadel instance, writing client credentials and tokens to Secrets.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/go-logr/logr"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	zclient "github.com/zitadel/zitadel-go/v3/pkg/client"
	"github.com/zitadel/zitadel-go/v3/pkg/client/middleware"
	"github.com/zitadel/zitadel-go/v3/pkg/zitadel"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/malpou/zitadel-operator/api/v1alpha1"
	"github.com/malpou/zitadel-operator/internal/controller"
)

type config struct {
	// Domain is the public Zitadel hostname, e.g. auth.example.com.
	Domain string `env:"ZITADEL_DOMAIN,required,notEmpty"`
	// KeyPath is the service-user JWT profile JSON (machine key).
	KeyPath string `env:"ZITADEL_KEY_PATH,required,notEmpty"`
	// OrgID is the organization every org-scoped kind lives in.
	OrgID string `env:"ZITADEL_ORG_ID,required,notEmpty"`
	// Namespace is the only namespace CRs are watched in.
	Namespace   string `env:"WATCH_NAMESPACE" envDefault:"zitadel-operator"`
	MetricsAddr string `env:"METRICS_ADDR"    envDefault:":8080"`
	HealthAddr  string `env:"HEALTH_ADDR"     envDefault:":8081"`
	// DryRun logs every write to Zitadel and refuses it. Use it to prove a
	// set of CRs matches what is already there before letting it loose.
	DryRun bool `env:"DRY_RUN" envDefault:"false"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctrl.SetLogger(logr.FromSlogHandler(logger.Handler()))
	if err := run(ctrl.SetupSignalHandler(), logger); err != nil {
		logger.Error("zitadel-operator exited", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := env.ParseAs[config]()
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	zc, err := zclient.New(
		ctx,
		zitadel.New(cfg.Domain),
		zclient.WithAuth(cachedAuth(cfg.KeyPath)),
		// v1 management calls take the org from this header; v2 calls carry it in the body.
		zclient.WithGRPCDialOptions(grpc.WithChainUnaryInterceptor(
			middleware.NewOrgInterceptor(cfg.OrgID).Unary(),
			writeAudit(logger, cfg.DryRun),
		)),
	)
	if err != nil {
		return fmt.Errorf("connect to zitadel: %w", err)
	}
	defer zc.Close()

	scheme := runtime.NewScheme()
	if err = clientgoscheme.AddToScheme(scheme); err != nil {
		return fmt.Errorf("register core types: %w", err)
	}
	if err = v1alpha1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("register zitadel types: %w", err)
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		// CRs live in one namespace; Secrets are written cluster-wide, so
		// keep them out of the informer cache instead of caching every
		// Secret in the cluster.
		Cache: cache.Options{DefaultNamespaces: map[string]cache.Config{cfg.Namespace: {}}},
		Client: client.Options{
			Cache: &client.CacheOptions{DisableFor: []client.Object{&corev1.Secret{}}},
		},
		Metrics:                metricsserver.Options{BindAddress: cfg.MetricsAddr},
		HealthProbeBindAddress: cfg.HealthAddr,
	})
	if err != nil {
		return fmt.Errorf("create manager: %w", err)
	}
	if err = mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("add healthz: %w", err)
	}
	if err = mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("add readyz: %w", err)
	}
	if err = controller.Setup(mgr, controller.Clients{
		OrgID:         cfg.OrgID,
		Project:       zc.ProjectServiceV2(),
		Application:   zc.ApplicationServiceV2(),
		Authorization: zc.AuthorizationServiceV2(),
		User:          zc.UserServiceV2(),
		Permission:    zc.InternalPermissionServiceV2(),
		Management:    zc.ManagementService(),
		Admin:         zc.AdminService(),
	}); err != nil {
		return fmt.Errorf("setup controllers: %w", err)
	}

	logger.InfoContext(ctx, "starting",
		"domain", cfg.Domain, "org", cfg.OrgID, "namespace", cfg.Namespace, "dryRun", cfg.DryRun)
	if err = mgr.Start(ctx); err != nil {
		return fmt.Errorf("run manager: %w", err)
	}

	return nil
}

var errDryRun = errors.New("dry run: write refused")

// cachedAuth reuses one access token until it expires. The SDK's own token
// source performs a full JWT profile exchange on every gRPC call, which the
// token endpoint rejects intermittently under concurrent reconciles.
func cachedAuth(keyPath string) zclient.TokenSourceInitializer {
	return func(ctx context.Context, issuer string) (oauth2.TokenSource, error) {
		src, err := zclient.DefaultServiceUserAuthentication(
			keyPath,
			oidc.ScopeOpenID,
			zclient.ScopeZitadelAPI(),
		)(
			ctx,
			issuer,
		)
		if err != nil {
			return nil, fmt.Errorf("jwt profile auth: %w", err)
		}

		return oauth2.ReuseTokenSource(nil, src), nil
	}
}

// writeAudit logs every mutating Zitadel call (anything but Get/List) and,
// in dry-run mode, refuses it.
func writeAudit(logger *slog.Logger, dryRun bool) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker, opts ...grpc.CallOption,
	) error {
		name := method[strings.LastIndex(method, "/")+1:]
		if strings.HasPrefix(name, "Get") || strings.HasPrefix(name, "List") {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		if dryRun {
			logger.WarnContext(ctx, "zitadel write refused", "method", method)

			return errDryRun
		}
		logger.InfoContext(ctx, "zitadel write", "method", method)

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
