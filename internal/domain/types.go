package domain

// RequestID identifies a request aggregate.
type RequestID string

// AllocationID identifies an allocation aggregate.
type AllocationID string

// ResourceID identifies a resource aggregate.
type ResourceID string

// ResourceClassID identifies a resource class.
type ResourceClassID string

// UserID identifies a user.
type UserID string

// RequestStatus represents the request lifecycle state.
type RequestStatus string

const (
	// RequestStatusOpen is the initial state of a request.
	RequestStatusOpen RequestStatus = "open"
	// RequestStatusInProgress indicates ongoing planning.
	RequestStatusInProgress RequestStatus = "in_progress"
	// RequestStatusPartiallyAllocated indicates partial assignment.
	RequestStatusPartiallyAllocated RequestStatus = "partially_allocated"
	// RequestStatusAllocated indicates full assignment.
	RequestStatusAllocated RequestStatus = "allocated"
	// RequestStatusActive indicates active execution.
	RequestStatusActive RequestStatus = "active"
	// RequestStatusCompleted indicates terminal completion.
	RequestStatusCompleted RequestStatus = "completed"
	// RequestStatusCancelled indicates terminal cancellation.
	RequestStatusCancelled RequestStatus = "cancelled"
)

// ExecutionState indicates executability of a request.
type ExecutionState string

const (
	// ExecutionStateExecutable indicates normal execution.
	ExecutionStateExecutable ExecutionState = "executable"
	// ExecutionStatePartiallyBlocked indicates partial blockage.
	ExecutionStatePartiallyBlocked ExecutionState = "partially_blocked"
	// ExecutionStateBlocked indicates full blockage.
	ExecutionStateBlocked ExecutionState = "blocked"
)

// AllocationStatus represents the allocation lifecycle state.
type AllocationStatus string

const (
	// AllocationStatusAllocated indicates initial allocation.
	AllocationStatusAllocated AllocationStatus = "allocated"
	// AllocationStatusShipped indicates dispatch shipment.
	AllocationStatusShipped AllocationStatus = "shipped"
	// AllocationStatusWithTechnician indicates technician possession.
	AllocationStatusWithTechnician AllocationStatus = "with_technician"
	// AllocationStatusReturnRequested indicates explicit return request.
	AllocationStatusReturnRequested AllocationStatus = "return_requested"
	// AllocationStatusShippedBack indicates shipment back to dispatch.
	AllocationStatusShippedBack AllocationStatus = "shipped_back"
	// AllocationStatusInspection indicates inspection phase.
	AllocationStatusInspection AllocationStatus = "inspection"
	// AllocationStatusCompleted indicates completed lifecycle.
	AllocationStatusCompleted AllocationStatus = "completed"
	// AllocationStatusCancelled indicates controlled cancellation.
	AllocationStatusCancelled AllocationStatus = "cancelled"
)

// ResourceStatus represents the resource lifecycle state.
type ResourceStatus string

const (
	// ResourceStatusAvailable indicates ready availability.
	ResourceStatusAvailable ResourceStatus = "available"
	// ResourceStatusReserved indicates planned reservation.
	ResourceStatusReserved ResourceStatus = "reserved"
	// ResourceStatusIssued indicates handed over by dispatch.
	ResourceStatusIssued ResourceStatus = "issued"
	// ResourceStatusInUse indicates active usage.
	ResourceStatusInUse ResourceStatus = "in_use"
	// ResourceStatusShippedBack indicates return shipment.
	ResourceStatusShippedBack ResourceStatus = "shipped_back"
	// ResourceStatusInspection indicates inspection state.
	ResourceStatusInspection ResourceStatus = "inspection"
	// ResourceStatusBlocked indicates blocked state.
	ResourceStatusBlocked ResourceStatus = "blocked"
	// ResourceStatusExternallyProcured indicates external procurement.
	ResourceStatusExternallyProcured ResourceStatus = "externally_procured"
)

// BlockReason defines why a resource is blocked.
type BlockReason string

const (
	// BlockReasonDefective indicates a defect.
	BlockReasonDefective BlockReason = "defective"
	// BlockReasonMaintenance indicates maintenance work.
	BlockReasonMaintenance BlockReason = "maintenance"
	// BlockReasonInspectionDue indicates overdue inspection.
	BlockReasonInspectionDue BlockReason = "inspection_due"
)
