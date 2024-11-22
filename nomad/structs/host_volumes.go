// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package structs

import (
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/nomad/helper"
	"github.com/hashicorp/nomad/helper/uuid"
)

type HostVolume struct {
	// Namespace is the Nomad namespace for the host volume, which constrains
	// which jobs can mount it.
	Namespace string

	// ID is a UUID-like string generated by the server.
	ID string

	// Name is the name that group.volume will use to identify the volume
	// source. Not expected to be unique.
	Name string

	// PluginID is the name of the host volume plugin on the client that will be
	// used for creating the volume. If omitted, the client will use its default
	// built-in plugin.
	PluginID string

	// NodePool is the node pool of the node where the volume is placed. If the
	// user doesn't provide a node ID, a node will be selected using the
	// NodePool and Constraints. If the user provides both NodePool and NodeID,
	// NodePool will be used to validate the request. If omitted, the server
	// will populate this value in before writing the volume to Raft.
	NodePool string

	// NodeID is the node where the volume is placed. If the user doesn't
	// provide a NodeID, one will be selected using the NodePool and
	// Constraints. If omitted, this field will then be populated by the server
	// before writing the volume to Raft.
	NodeID string

	// Constraints are optional. If the NodeID is not provided, the NodePool and
	// Constraints are used to select a node. If the NodeID is provided,
	// Constraints are used to validate that the node meets those constraints at
	// the time of volume creation.
	Constraints []*Constraint `json:",omitempty"`

	// Because storage may allow only specific intervals of size, we accept a
	// min and max and return the actual capacity when the volume is created or
	// updated on the client
	RequestedCapacityMinBytes int64
	RequestedCapacityMaxBytes int64
	CapacityBytes             int64

	// RequestedCapabilities defines the options available to group.volume
	// blocks. The scheduler checks against the listed capability blocks and
	// selects a node for placement if *any* capability block works.
	RequestedCapabilities []*HostVolumeCapability

	// Parameters are an opaque map of parameters for the host volume plugin.
	Parameters map[string]string `json:",omitempty"`

	// HostPath is the path on disk where the volume's mount point was
	// created. We record this to make debugging easier.
	HostPath string

	// State represents the overall state of the volume. One of pending, ready,
	// deleted.
	State HostVolumeState

	CreateIndex uint64
	CreateTime  int64 // Unix timestamp in nanoseconds since epoch

	ModifyIndex uint64
	ModifyTime  int64 // Unix timestamp in nanoseconds since epoch

	// Allocations is the list of non-client-terminal allocations with claims on
	// this host volume. They are denormalized on read and this field will be
	// never written to Raft
	Allocations []*AllocListStub `json:",omitempty"`
}

type HostVolumeState string

const (
	HostVolumeStateUnknown HostVolumeState = "" // never write this to Raft
	HostVolumeStatePending HostVolumeState = "pending"
	HostVolumeStateReady   HostVolumeState = "ready"
	HostVolumeStateDeleted HostVolumeState = "deleted"
)

func (hv *HostVolume) Copy() *HostVolume {
	if hv == nil {
		return nil
	}

	nhv := *hv
	nhv.Constraints = helper.CopySlice(hv.Constraints)
	nhv.RequestedCapabilities = helper.CopySlice(hv.RequestedCapabilities)
	nhv.Parameters = maps.Clone(hv.Parameters)
	return &nhv
}

func (hv *HostVolume) Stub() *HostVolumeStub {
	if hv == nil {
		return nil
	}

	return &HostVolumeStub{
		Namespace:     hv.Namespace,
		ID:            hv.ID,
		Name:          hv.Name,
		PluginID:      hv.PluginID,
		NodePool:      hv.NodePool,
		NodeID:        hv.NodeID,
		CapacityBytes: hv.CapacityBytes,
		State:         hv.State,
		CreateIndex:   hv.CreateIndex,
		CreateTime:    hv.CreateTime,
		ModifyIndex:   hv.ModifyIndex,
		ModifyTime:    hv.ModifyTime,
	}
}

// Validate verifies that the submitted HostVolume spec has valid field values,
// without validating any changes or state (see ValidateUpdate).
func (hv *HostVolume) Validate() error {

	var mErr *multierror.Error

	if hv.ID != "" && !helper.IsUUID(hv.ID) {
		mErr = multierror.Append(mErr, errors.New("invalid ID"))
	}

	if hv.Name == "" {
		mErr = multierror.Append(mErr, errors.New("missing name"))
	}

	if hv.RequestedCapacityMaxBytes < hv.RequestedCapacityMinBytes {
		mErr = multierror.Append(mErr, fmt.Errorf(
			"capacity_max (%d) must be larger than capacity_min (%d)",
			hv.RequestedCapacityMaxBytes, hv.RequestedCapacityMinBytes))
	}

	if len(hv.RequestedCapabilities) == 0 {
		mErr = multierror.Append(mErr, errors.New("must include at least one capability block"))
	} else {
		for _, cap := range hv.RequestedCapabilities {
			err := cap.Validate()
			if err != nil {
				mErr = multierror.Append(mErr, err)
			}
		}
	}

	for _, constraint := range hv.Constraints {
		if err := constraint.Validate(); err != nil {
			mErr = multierror.Append(mErr, fmt.Errorf("invalid constraint: %v", err))
		}
		switch constraint.Operand {
		case ConstraintDistinctHosts, ConstraintDistinctProperty:
			mErr = multierror.Append(mErr, fmt.Errorf(
				"invalid constraint %s: host volumes of the same name are always on distinct hosts", constraint.Operand))
		default:
		}
	}

	return helper.FlattenMultierror(mErr.ErrorOrNil())
}

// ValidateUpdate verifies that an update to a volume is safe to make.
func (hv *HostVolume) ValidateUpdate(existing *HostVolume) error {
	if existing == nil {
		return nil
	}

	var mErr *multierror.Error
	if len(existing.Allocations) > 0 {
		allocIDs := helper.ConvertSlice(existing.Allocations,
			func(a *AllocListStub) string { return a.ID })
		mErr = multierror.Append(mErr, fmt.Errorf(
			"cannot update a volume in use: claimed by allocs (%s)",
			strings.Join(allocIDs, ", ")))
	}

	if hv.NodeID != "" && hv.NodeID != existing.NodeID {
		mErr = multierror.Append(mErr, errors.New("node ID cannot be updated"))
	}
	if hv.NodePool != "" && hv.NodePool != existing.NodePool {
		mErr = multierror.Append(mErr, errors.New("node pool cannot be updated"))
	}

	if hv.RequestedCapacityMaxBytes < existing.CapacityBytes {
		mErr = multierror.Append(mErr, fmt.Errorf(
			"capacity_max (%d) cannot be less than existing provisioned capacity (%d)",
			hv.RequestedCapacityMaxBytes, existing.CapacityBytes))
	}

	return mErr.ErrorOrNil()
}

const DefaultHostVolumePlugin = "default"

// CanonicalizeForUpdate is called in the RPC handler to ensure we call client
// RPCs with correctly populated fields from the existing volume, even if the
// RPC request includes otherwise valid zero-values. This method should be
// called on request objects or a copy, never on a state store object directly.
func (hv *HostVolume) CanonicalizeForUpdate(existing *HostVolume, now time.Time) {
	if existing == nil {
		hv.ID = uuid.Generate()
		if hv.PluginID == "" {
			hv.PluginID = DefaultHostVolumePlugin
		}
		hv.CapacityBytes = 0 // returned by plugin
		hv.HostPath = ""     // returned by plugin
		hv.CreateTime = now.UnixNano()
	} else {
		hv.PluginID = existing.PluginID
		hv.NodePool = existing.NodePool
		hv.NodeID = existing.NodeID
		hv.Constraints = existing.Constraints
		hv.CapacityBytes = existing.CapacityBytes
		hv.HostPath = existing.HostPath
		hv.CreateTime = existing.CreateTime
	}

	hv.State = HostVolumeStatePending // reset on any change
	hv.ModifyTime = now.UnixNano()
	hv.Allocations = nil // set on read only
}

// GetNamespace implements the paginator.NamespaceGetter interface
func (hv *HostVolume) GetNamespace() string {
	return hv.Namespace
}

// GetID implements the paginator.IDGetter interface
func (hv *HostVolume) GetID() string {
	return hv.ID
}

// HostVolumeCapability is the requested attachment and access mode for a volume
type HostVolumeCapability struct {
	AttachmentMode HostVolumeAttachmentMode
	AccessMode     HostVolumeAccessMode
}

func (hvc *HostVolumeCapability) Copy() *HostVolumeCapability {
	if hvc == nil {
		return nil
	}

	nhvc := *hvc
	return &nhvc
}

func (hvc *HostVolumeCapability) Validate() error {
	if hvc == nil {
		return errors.New("validate called on nil host volume capability")
	}

	switch hvc.AttachmentMode {
	case HostVolumeAttachmentModeBlockDevice,
		HostVolumeAttachmentModeFilesystem:
	default:
		return fmt.Errorf("invalid attachment mode: %q", hvc.AttachmentMode)
	}

	switch hvc.AccessMode {
	case HostVolumeAccessModeSingleNodeReader,
		HostVolumeAccessModeSingleNodeWriter,
		HostVolumeAccessModeMultiNodeReader,
		HostVolumeAccessModeMultiNodeSingleWriter,
		HostVolumeAccessModeMultiNodeMultiWriter:
	default:
		return fmt.Errorf("invalid access mode: %q", hvc.AccessMode)
	}

	return nil
}

// HostVolumeAttachmentMode chooses the type of storage API that will be used to
// interact with the device.
type HostVolumeAttachmentMode string

const (
	HostVolumeAttachmentModeUnknown     HostVolumeAttachmentMode = ""
	HostVolumeAttachmentModeBlockDevice HostVolumeAttachmentMode = "block-device"
	HostVolumeAttachmentModeFilesystem  HostVolumeAttachmentMode = "file-system"
)

// HostVolumeAccessMode indicates how Nomad should make the volume available to
// concurrent allocations.
type HostVolumeAccessMode string

const (
	HostVolumeAccessModeUnknown HostVolumeAccessMode = ""

	HostVolumeAccessModeSingleNodeReader HostVolumeAccessMode = "single-node-reader-only"
	HostVolumeAccessModeSingleNodeWriter HostVolumeAccessMode = "single-node-writer"

	HostVolumeAccessModeMultiNodeReader       HostVolumeAccessMode = "multi-node-reader-only"
	HostVolumeAccessModeMultiNodeSingleWriter HostVolumeAccessMode = "multi-node-single-writer"
	HostVolumeAccessModeMultiNodeMultiWriter  HostVolumeAccessMode = "multi-node-multi-writer"
)

// HostVolumeStub is used for responses for the list volumes endpoint
type HostVolumeStub struct {
	Namespace     string
	ID            string
	Name          string
	PluginID      string
	NodePool      string
	NodeID        string
	CapacityBytes int64
	State         HostVolumeState

	CreateIndex uint64
	CreateTime  int64

	ModifyIndex uint64
	ModifyTime  int64
}

type HostVolumeCreateRequest struct {
	Volumes []*HostVolume
	WriteRequest
}

type HostVolumeCreateResponse struct {
	Volumes []*HostVolume
	WriteMeta
}

type HostVolumeRegisterRequest struct {
	Volumes []*HostVolume
	WriteRequest
}

type HostVolumeRegisterResponse struct {
	Volumes []*HostVolume
	WriteMeta
}

type HostVolumeDeleteRequest struct {
	VolumeIDs []string
	WriteRequest
}

type HostVolumeDeleteResponse struct {
	VolumeIDs []string // volumes actually deleted
	WriteMeta
}

type HostVolumeGetRequest struct {
	ID string
	QueryOptions
}

type HostVolumeGetResponse struct {
	Volume *HostVolume
	QueryMeta
}

type HostVolumeListRequest struct {
	NodeID   string // filter
	NodePool string // filter
	QueryOptions
}

type HostVolumeListResponse struct {
	Volumes []*HostVolumeStub
	QueryMeta
}
