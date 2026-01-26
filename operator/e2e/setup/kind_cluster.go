// /*
// Copyright 2025 The Grove Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// */

package setup

import (
	"context"
	"fmt"
	"os"

	"github.com/ai-dynamo/grove/operator/e2e"
	"github.com/ai-dynamo/grove/operator/e2e/utils"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// SetupCompleteKindCluster sets up a Kind cluster that's already been created externally
// (e.g., by GitHub Actions helm/kind-action) with Grove, Kai Scheduler, and necessary components.
// This is designed to work with pre-created Kind clusters in CI environments.
func SetupCompleteKindCluster(ctx context.Context, clusterName string, skaffoldYAMLPath string, logger *utils.Logger) (*rest.Config, func(), error) {
	// Get kubeconfig from default location or KUBECONFIG env var
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		kubeconfigPath = fmt.Sprintf("%s/.kube/config", homeDir)
	}

	logger.Infof("📝 Using kubeconfig from: %s", kubeconfigPath)

	// Load the kubeconfig
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
	}

	// Create clientset for node monitoring
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("could not create clientset: %w", err)
	}

	// Start node monitoring to handle not ready nodes
	nodeMonitoringCleanup := StartNodeMonitoring(ctx, clientset, logger)

	// Create cleanup function
	cleanup := func() {
		// Stop node monitoring
		nodeMonitoringCleanup()
		logger.Info("🧹 Cleanup complete (cluster will be deleted by CI)")
	}

	// Load dependencies for image pre-pulling and helm charts
	deps, err := e2e.GetDependencies()
	if err != nil {
		return nil, cleanup, fmt.Errorf("failed to load dependencies: %w", err)
	}

	// Note: Kind clusters in CI typically don't need manual image pre-pulling
	// as the helm/kind-action handles Docker image loading automatically

	tolerations := []map[string]interface{}{
		{
			"key":      "node-role.kubernetes.io/control-plane",
			"operator": "Exists",
			"effect":   "NoSchedule",
		},
	}

	// Use Kai Scheduler configuration from dependencies
	kaiConfig := &HelmInstallConfig{
		ReleaseName:     deps.HelmCharts.KaiScheduler.ReleaseName,
		ChartRef:        deps.HelmCharts.KaiScheduler.ChartRef,
		ChartVersion:    deps.HelmCharts.KaiScheduler.Version,
		Namespace:       deps.HelmCharts.KaiScheduler.Namespace,
		RestConfig:      restConfig,
		CreateNamespace: true,
		Wait:            false,
		Values: map[string]interface{}{
			"global": map[string]interface{}{
				"tolerations": tolerations,
			},
		},
		HelmLoggerFunc: logger.Debugf,
		Logger:         logger,
	}

	logger.Info("🚀 Installing Grove, Kai Scheduler...")
	if err := InstallCoreComponents(ctx, restConfig, kaiConfig, skaffoldYAMLPath, "", logger); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("component installation failed: %w", err)
	}

	// Wait for Grove pods to be ready (0 = skip count validation)
	if err := utils.WaitForPodsInNamespace(ctx, OperatorNamespace, restConfig, 0, defaultPollTimeout, defaultPollInterval, logger); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("grove pods not ready: %w", err)
	}

	// Wait for Kai Scheduler pods to be ready (0 = skip count validation)
	if err := utils.WaitForPodsInNamespace(ctx, kaiConfig.Namespace, kaiConfig.RestConfig, 0, defaultPollTimeout, defaultPollInterval, kaiConfig.Logger); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("kai scheduler pods not ready: %w", err)
	}

	// Wait for the Kai CRDs to be available (before creating queues)
	if err := WaitForKaiCRDs(ctx, kaiConfig); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("failed to wait for Kai CRDs: %w", err)
	}

	// Create the default Kai queues
	if err := CreateDefaultKaiQueues(ctx, kaiConfig); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("failed to create default Kai queue: %w", err)
	}

	// Wait for Grove webhook to be ready by actually testing it
	if err := waitForWebhookReady(ctx, restConfig, logger); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("grove webhook not ready: %w", err)
	}

	logger.Info("✅ Kind cluster setup complete with Grove and Kai Scheduler")
	return restConfig, cleanup, nil
}
