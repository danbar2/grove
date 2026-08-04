// Copyright 2026 The Grove Authors.
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

package podclique

import (
	"context"
	"testing"

	grovecorev1alpha1 "github.com/ai-dynamo/grove/operator/api/core/v1alpha1"
	testutils "github.com/ai-dynamo/grove/operator/test/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/uuid"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestReconcileReadsThePodCliqueFromTheAPIServer asserts the reconciled object comes from apiReader
// and not the cache (issue #711). The two readers are given divergent contents, so only a reconcile
// reading through apiReader sees the PodClique as absent and stops.
func TestReconcileReadsThePodCliqueFromTheAPIServer(t *testing.T) {
	ctx := context.Background()
	pclq := testutils.NewPodCliqueBuilder(testPCSName, uuid.NewUUID(), "worker", testNamespace, 0).Build()
	pclqObjectKey := client.ObjectKeyFromObject(pclq)

	cachedReader := testutils.CreateDefaultFakeClient([]client.Object{pclq})
	liveReader := testutils.CreateDefaultFakeClient(nil)
	r := &Reconciler{client: cachedReader, apiReader: liveReader}

	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: pclqObjectKey})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// ensureFinalizer runs first once a reconcile has an object, so an untouched finalizer list means
	// the cached copy never drove one.
	stored := &grovecorev1alpha1.PodClique{}
	require.NoError(t, cachedReader.Get(ctx, pclqObjectKey, stored))
	assert.Empty(t, stored.Finalizers, "reconcile must not proceed on the cached PodClique")
}
