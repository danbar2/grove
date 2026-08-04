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

package podcliquescalinggroup

import (
	"context"
	"testing"

	grovecorev1alpha1 "github.com/ai-dynamo/grove/operator/api/core/v1alpha1"
	testutils "github.com/ai-dynamo/grove/operator/test/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestReconcileReadsThePodCliqueScalingGroupFromTheAPIServer asserts the reconciled object comes from
// apiReader and not the cache (issue #711). The two readers are given divergent contents, so only a
// reconcile reading through apiReader sees the PodCliqueScalingGroup as absent and stops.
func TestReconcileReadsThePodCliqueScalingGroupFromTheAPIServer(t *testing.T) {
	ctx := context.Background()
	pcsg := testutils.NewPodCliqueScalingGroupBuilder(testPCSGName, testNamespacePCSG, testPCSNamePCSG, 0).Build()
	pcsgObjectKey := client.ObjectKeyFromObject(pcsg)

	cachedReader := testutils.CreateDefaultFakeClient([]client.Object{pcsg})
	liveReader := testutils.CreateDefaultFakeClient(nil)
	r := &Reconciler{client: cachedReader, apiReader: liveReader}

	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: pcsgObjectKey})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// ensureFinalizer runs first once a reconcile has an object, so an untouched finalizer list means
	// the cached copy never drove one.
	stored := &grovecorev1alpha1.PodCliqueScalingGroup{}
	require.NoError(t, cachedReader.Get(ctx, pcsgObjectKey, stored))
	assert.Empty(t, stored.Finalizers, "reconcile must not proceed on the cached PodCliqueScalingGroup")
}
