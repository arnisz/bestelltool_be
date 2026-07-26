package domain

const (
	// PermissionRequestCreate permits creating resource requests.
	PermissionRequestCreate = "request.create"
	// PermissionRequestRead permits reading resource requests.
	PermissionRequestRead = "request.read"
	// PermissionAllocationReturnRequest permits requesting the return of an allocation.
	PermissionAllocationReturnRequest = "allocation.return_request"
	// PermissionResourceTransferDirect permits directly transferring a resource.
	PermissionResourceTransferDirect = "resource.transfer_direct"
	// PermissionEventStreamOwn permits subscribing to the actor's own operational events.
	PermissionEventStreamOwn = "event.stream.own"
	// PermissionEventStreamAll permits subscribing to all operational events.
	PermissionEventStreamAll = "event.stream.all"
)
