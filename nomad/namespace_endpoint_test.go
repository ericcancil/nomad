// +build pro ent

package nomad

import (
	"testing"
	"time"

	msgpackrpc "github.com/hashicorp/net-rpc-msgpackrpc"
	"github.com/hashicorp/nomad/nomad/mock"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/testutil"
	"github.com/stretchr/testify/assert"
)

func TestNamespaceEndpoint_GetNamespace(t *testing.T) {
	assert := assert.New(t)
	t.Parallel()
	s1 := testServer(t, nil)
	defer s1.Shutdown()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the register request
	ns := mock.Namespace()
	s1.fsm.State().UpsertNamespaces(1000, []*structs.Namespace{ns})

	// Lookup the policy
	get := &structs.NamespaceSpecificRequest{
		Name:         ns.Name,
		QueryOptions: structs.QueryOptions{Region: "global"},
	}
	var resp structs.SingleNamespaceResponse
	assert.Nil(msgpackrpc.CallWithCodec(codec, "Namespace.GetNamespace", get, &resp))
	assert.EqualValues(1000, resp.Index)
	assert.Equal(ns, resp.Namespace)

	// Lookup non-existing policy
	get.Name = structs.GenerateUUID()
	assert.Nil(msgpackrpc.CallWithCodec(codec, "Namespace.GetNamespace", get, &resp))
	assert.EqualValues(1000, resp.Index)
	assert.Nil(resp.Namespace)
}

func TestNamespaceEndpoint_GetNamespace_Blocking(t *testing.T) {
	assert := assert.New(t)
	t.Parallel()
	s1 := testServer(t, nil)
	defer s1.Shutdown()
	state := s1.fsm.State()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the namespaces
	ns1 := mock.Namespace()
	ns2 := mock.Namespace()

	// First create an namespace
	time.AfterFunc(100*time.Millisecond, func() {
		assert.Nil(state.UpsertNamespaces(100, []*structs.Namespace{ns1}))
	})

	// Upsert the namespace we are watching later
	time.AfterFunc(200*time.Millisecond, func() {
		assert.Nil(state.UpsertNamespaces(200, []*structs.Namespace{ns2}))
	})

	// Lookup the policy
	req := &structs.NamespaceSpecificRequest{
		Name: ns2.Name,
		QueryOptions: structs.QueryOptions{
			Region:        "global",
			MinQueryIndex: 150,
		},
	}
	var resp structs.SingleNamespaceResponse
	start := time.Now()
	assert.Nil(msgpackrpc.CallWithCodec(codec, "Namespace.GetNamespace", req, &resp))
	assert.EqualValues(200, resp.Index)
	assert.NotNil(resp.Namespace)
	assert.Equal(ns2.Name, resp.Namespace.Name)

	if elapsed := time.Since(start); elapsed < 200*time.Millisecond {
		t.Fatalf("should block (returned in %s) %#v", elapsed, resp)
	}

	// Namespace delete triggers watches
	time.AfterFunc(100*time.Millisecond, func() {
		assert.Nil(state.DeleteNamespaces(300, []string{ns2.Name}))
	})

	req.QueryOptions.MinQueryIndex = 250
	var resp2 structs.SingleNamespaceResponse
	start = time.Now()
	assert.Nil(msgpackrpc.CallWithCodec(codec, "Namespace.GetNamespace", req, &resp2))
	assert.EqualValues(300, resp2.Index)
	assert.Nil(resp2.Namespace)

	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("should block (returned in %s) %#v", elapsed, resp2)
	}
}

func TestNamespaceEndpoint_List(t *testing.T) {
	assert := assert.New(t)
	t.Parallel()
	s1 := testServer(t, nil)
	defer s1.Shutdown()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the register request
	ns1 := mock.Namespace()
	ns2 := mock.Namespace()

	ns1.Name = "aaaaaaaa-3350-4b4b-d185-0e1992ed43e9"
	ns2.Name = "aaaabbbb-3350-4b4b-d185-0e1992ed43e9"
	assert.Nil(s1.fsm.State().UpsertNamespaces(1000, []*structs.Namespace{ns1, ns2}))

	// Lookup the namespaces
	get := &structs.NamespaceListRequest{
		QueryOptions: structs.QueryOptions{Region: "global"},
	}
	var resp structs.NamespaceListResponse
	assert.Nil(msgpackrpc.CallWithCodec(codec, "Namespace.ListNamespaces", get, &resp))
	assert.EqualValues(1000, resp.Index)
	assert.Len(resp.Namespaces, 2)

	// Lookup the namespaces by prefix
	get = &structs.NamespaceListRequest{
		QueryOptions: structs.QueryOptions{
			Region: "global",
			Prefix: "aaaabb",
		},
	}
	var resp2 structs.NamespaceListResponse
	assert.Nil(msgpackrpc.CallWithCodec(codec, "Namespace.ListNamespaces", get, &resp2))
	assert.EqualValues(1000, resp2.Index)
	assert.Len(resp2.Namespaces, 1)
}

func TestNamespaceEndpoint_List_Blocking(t *testing.T) {
	assert := assert.New(t)
	t.Parallel()
	s1 := testServer(t, nil)
	defer s1.Shutdown()
	state := s1.fsm.State()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the namespace
	ns := mock.Namespace()

	// Upsert namespace triggers watches
	time.AfterFunc(100*time.Millisecond, func() {
		assert.Nil(state.UpsertNamespaces(2, []*structs.Namespace{ns}))
	})

	req := &structs.NamespaceListRequest{
		QueryOptions: structs.QueryOptions{
			Region:        "global",
			MinQueryIndex: 1,
		},
	}
	start := time.Now()
	var resp structs.NamespaceListResponse
	assert.Nil(msgpackrpc.CallWithCodec(codec, "Namespace.ListNamespaces", req, &resp))

	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("should block (returned in %s) %#v", elapsed, resp)
	}
	assert.EqualValues(2, resp.Index)
	assert.Len(resp.Namespaces, 1)
	assert.Equal(ns.Name, resp.Namespaces[0].Name)

	// Namespace deletion triggers watches
	time.AfterFunc(100*time.Millisecond, func() {
		assert.Nil(state.DeleteNamespaces(3, []string{ns.Name}))
	})

	req.MinQueryIndex = 2
	start = time.Now()
	var resp2 structs.NamespaceListResponse
	assert.Nil(msgpackrpc.CallWithCodec(codec, "Namespace.ListNamespaces", req, &resp2))

	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("should block (returned in %s) %#v", elapsed, resp2)
	}
	assert.EqualValues(3, resp2.Index)
	assert.Len(resp2.Namespaces, 0)
}

func TestNamespaceEndpoint_DeleteNamespaces(t *testing.T) {
	assert := assert.New(t)
	t.Parallel()
	s1 := testServer(t, nil)
	defer s1.Shutdown()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the register request
	ns1 := mock.Namespace()
	ns2 := mock.Namespace()
	s1.fsm.State().UpsertNamespaces(1000, []*structs.Namespace{ns1, ns2})

	// Lookup the policies
	req := &structs.NamespaceDeleteRequest{
		Namespaces:   []string{ns1.Name, ns2.Name},
		WriteRequest: structs.WriteRequest{Region: "global"},
	}
	var resp structs.GenericResponse
	assert.Nil(msgpackrpc.CallWithCodec(codec, "Namespace.DeleteNamespaces", req, &resp))
	assert.NotEqual(uint64(0), resp.Index)
}

func TestNamespaceEndpoint_DeleteNamespaces_Default(t *testing.T) {
	assert := assert.New(t)
	t.Parallel()
	s1 := testServer(t, nil)
	defer s1.Shutdown()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Delete the default namespace
	req := &structs.NamespaceDeleteRequest{
		Namespaces:   []string{structs.DefaultNamespace},
		WriteRequest: structs.WriteRequest{Region: "global"},
	}
	var resp structs.GenericResponse
	assert.NotNil(msgpackrpc.CallWithCodec(codec, "Namespace.DeleteNamespaces", req, &resp))
}

func TestNamespaceEndpoint_UpsertNamespaces(t *testing.T) {
	assert := assert.New(t)
	t.Parallel()
	s1 := testServer(t, nil)
	defer s1.Shutdown()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the register request
	ns1 := mock.Namespace()
	ns2 := mock.Namespace()

	// Lookup the policies
	req := &structs.NamespaceUpsertRequest{
		Namespaces:   []*structs.Namespace{ns1, ns2},
		WriteRequest: structs.WriteRequest{Region: "global"},
	}
	var resp structs.GenericResponse
	assert.Nil(msgpackrpc.CallWithCodec(codec, "Namespace.UpsertNamespaces", req, &resp))
	assert.NotEqual(uint64(0), resp.Index)

	// Check we created the namespaces
	out, err := s1.fsm.State().NamespaceByName(nil, ns1.Name)
	assert.Nil(err)
	assert.NotNil(out)

	out, err = s1.fsm.State().NamespaceByName(nil, ns2.Name)
	assert.Nil(err)
	assert.NotNil(out)
}