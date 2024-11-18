package runtime

import (
	"time"

	"github.com/kyma-project/control-plane/components/provisioner/pkg/gqlschema"
)

type State string

const (
	// StateSucceeded means that the last operation of the runtime has succeeded.
	StateSucceeded State = "succeeded"
	// StateFailed means that the last operation is one of provision, deprovivion, suspension, unsuspension, which has failed.
	StateFailed State = "failed"
	// StateError means the runtime is in a recoverable error state, due to the last upgrade/update operation has failed.
	StateError State = "error"
	// StateProvisioning means that the runtime provisioning (or unsuspension) is in progress (by the last runtime operation).
	StateProvisioning State = "provisioning"
	// StateDeprovisioning means that the runtime deprovisioning (or suspension) is in progress (by the last runtime operation).
	StateDeprovisioning State = "deprovisioning"
	// StateDeprovisioned means that the runtime deprovisioning has finished removing the instance.
	// In case the instance has already been deleted, KEB will try best effort to reconstruct at least partial information regarding deprovisioned instances from residual operations.
	StateDeprovisioned State = "deprovisioned"
	// StateDeprovisionIncomplete means that the runtime deprovisioning has finished removing the instance but certain steps have not finished and the instance should be requeued for repeated deprovisioning.
	StateDeprovisionIncomplete State = "deprovisionincomplete"
	// StateUpgrading means that kyma upgrade or cluster upgrade operation is in progress.
	StateUpgrading State = "upgrading"
	// StateUpdating means the runtime configuration is being updated (i.e. OIDC is reconfigured).
	StateUpdating State = "updating"
	// StateSuspended means that the trial runtime is suspended (i.e. deprovisioned).
	StateSuspended State = "suspended"
	// AllState is a virtual state only used as query parameter in ListParameters to indicate "include all runtimes, which are excluded by default without state filters".
	AllState State = "all"
)

type RuntimeDTO struct {
	InstanceID                  string                         `json:"instanceID"`
	RuntimeID                   string                         `json:"runtimeID"`
	GlobalAccountID             string                         `json:"globalAccountID"`
	SubscriptionGlobalAccountID string                         `json:"subscriptionGlobalAccountID"`
	SubAccountID                string                         `json:"subAccountID"`
	ProviderRegion              string                         `json:"region"`
	SubAccountRegion            string                         `json:"subAccountRegion"`
	ShootName                   string                         `json:"shootName"`
	ServiceClassID              string                         `json:"serviceClassID"`
	ServiceClassName            string                         `json:"serviceClassName"`
	ServicePlanID               string                         `json:"servicePlanID"`
	ServicePlanName             string                         `json:"servicePlanName"`
	Provider                    string                         `json:"provider"`
	Parameters                  ProvisioningParameters         `json:"provisioningParameters,omitempty"`
	Status                      RuntimeStatus                  `json:"status"`
	UserID                      string                         `json:"userID"`
	KymaConfig                  *gqlschema.KymaConfigInput     `json:"kymaConfig,omitempty"`
	ClusterConfig               *gqlschema.GardenerConfigInput `json:"clusterConfig,omitempty"`
	RuntimeConfig               *map[string]interface{}        `json:"runtimeConfig,omitempty"`
	Bindings                    []BindingDTO                   `json:"bindings,omitempty"`
}

type ProvisioningParameters struct {
	Parameters ProvisioningParametersDTO `json:"parameters"`
}
type ProvisioningParametersDTO struct {
	AutoScalerParameters  *AutoScalerParametersDTO `json:"autoScalerParameters,omitempty"`
	Name                  string                   `json:"name"`
	TargetSecret          *string                  `json:"targetSecret,omitempty"`
	VolumeSizeGb          *int                     `json:"volumeSizeGb,omitempty"`
	MachineType           *string                  `json:"machineType,omitempty"`
	Region                *string                  `json:"region,omitempty"`
	Purpose               *string                  `json:"purpose,omitempty"`
	LicenceType           *string                  `json:"licence_type,omitempty"`
	Zones                 []string                 `json:"zones,omitempty"`
	RuntimeAdministrators []string                 `json:"administrators,omitempty"`
	ShootName             string                   `json:"shootName,omitempty"`
	ShootDomain           string                   `json:"shootDomain,omitempty"`

	OIDC                   *OIDCConfigDTO `json:"oidc,omitempty"`
	Networking             *NetworkingDTO `json:"networking,omitempty"`
	ShootAndSeedSameRegion *bool          `json:"shootAndSeedSameRegion,omitempty"`
}

type AutoScalerParametersDTO struct {
	AutoScalerMin  *int `json:"autoScalerMin,omitempty"`
	AutoScalerMax  *int `json:"autoScalerMax,omitempty"`
	MaxSurge       *int `json:"maxSurge,omitempty"`
	MaxUnavailable *int `json:"maxUnavailable,omitempty"`
}

type OIDCConfigDTO struct {
	ClientID       string   `json:"clientID" yaml:"clientID"`
	GroupsClaim    string   `json:"groupsClaim" yaml:"groupsClaim"`
	IssuerURL      string   `json:"issuerURL" yaml:"issuerURL"`
	SigningAlgs    []string `json:"signingAlgs" yaml:"signingAlgs"`
	UsernameClaim  string   `json:"usernameClaim" yaml:"usernameClaim"`
	UsernamePrefix string   `json:"usernamePrefix" yaml:"usernamePrefix"`
}

type NetworkingDTO struct {
	NodesCidr    string  `json:"nodes,omitempty"`
	PodsCidr     *string `json:"pods,omitempty"`
	ServicesCidr *string `json:"services,omitempty"`
}

type BindingDTO struct {
	ID                string    `json:"id"`
	CreatedAt         time.Time `json:"createdAt"`
	ExpirationSeconds int64     `json:"expiresInSeconds"`
	ExpiresAt         time.Time `json:"expiresAt"`
	KubeconfigExists  bool      `json:"kubeconfigExists"`
	CreatedBy         string    `json:"createdBy"`
}

type RuntimeStatus struct {
	CreatedAt        time.Time                 `json:"createdAt"`
	ModifiedAt       time.Time                 `json:"modifiedAt"`
	ExpiredAt        *time.Time                `json:"expiredAt,omitempty"`
	DeletedAt        *time.Time                `json:"deletedAt,omitempty"`
	State            State                     `json:"state"`
	Provisioning     *Operation                `json:"provisioning,omitempty"`
	Deprovisioning   *Operation                `json:"deprovisioning,omitempty"`
	UpgradingCluster *OperationsData           `json:"upgradingCluster,omitempty"`
	Update           *OperationsData           `json:"update,omitempty"`
	Suspension       *OperationsData           `json:"suspension,omitempty"`
	Unsuspension     *OperationsData           `json:"unsuspension,omitempty"`
	GardenerConfig   *gqlschema.GardenerConfig `json:"gardenerConfig,omitempty"`
}

type OperationType string

const (
	Provision      OperationType = "provision"
	Deprovision    OperationType = "deprovision"
	UpgradeCluster OperationType = "cluster upgrade"
	Update         OperationType = "update"
	Suspension     OperationType = "suspension"
	Unsuspension   OperationType = "unsuspension"
)

type OperationsData struct {
	Data       []Operation `json:"data"`
	TotalCount int         `json:"totalCount"`
	Count      int         `json:"count"`
}

type Operation struct {
	State                        string                 `json:"state"`
	Type                         OperationType          `json:"type,omitempty"`
	Description                  string                 `json:"description"`
	CreatedAt                    time.Time              `json:"createdAt"`
	UpdatedAt                    time.Time              `json:"updatedAt"`
	OperationID                  string                 `json:"operationID"`
	OrchestrationID              string                 `json:"orchestrationID,omitempty"`
	FinishedStages               []string               `json:"finishedStages"`
	ExecutedButNotCompletedSteps []string               `json:"executedButNotCompletedSteps,omitempty"`
	ProvisioningParameters       ProvisioningParameters `json:"provisioningParameters,omitempty"`
	//parameters
}

type RuntimesPage struct {
	Data       []RuntimeDTO `json:"data"`
	Count      int          `json:"count"`
	TotalCount int          `json:"totalCount"`
}

const (
	GlobalAccountIDParam = "account"
	SubAccountIDParam    = "subaccount"
	InstanceIDParam      = "instance_id"
	RuntimeIDParam       = "runtime_id"
	RegionParam          = "region"
	ShootParam           = "shoot"
	PlanParam            = "plan"
	StateParam           = "state"
	OperationDetailParam = "op_detail"
	KymaConfigParam      = "kyma_config"
	ClusterConfigParam   = "cluster_config"
	ExpiredParam         = "expired"
	GardenerConfigParam  = "gardener_config"
	RuntimeConfigParam   = "runtime_config"
	BindingsParam        = "bindings"
	WithBindingsParam    = "with_bindings"
)

type OperationDetail string

const (
	LastOperation OperationDetail = "last"
	AllOperation  OperationDetail = "all"
)

type ListParameters struct {
	// Page specifies the offset for the runtime results in the total count of matching runtimes
	Page int
	// PageSize specifies the count of matching runtimes returned in a response
	PageSize int
	// OperationDetail specifies whether the server should respond with all operations, or only the last operation. If not set, the server by default sends all operations
	OperationDetail OperationDetail
	// KymaConfig specifies whether kyma configuration details should be included in the response for each runtime
	KymaConfig bool
	// ClusterConfig specifies whether Gardener cluster configuration details should be included in the response for each runtime
	ClusterConfig bool
	// RuntimeResourceConfig specifies whether current Runtime Custom Resource details should be included in the response for each runtime
	RuntimeResourceConfig bool
	// Bindings specifies whether runtime bindings should be included in the response for each runtime
	Bindings bool
	// WithBindings parameter filters runtimes to show only those with bindings
	WithBindings bool
	// GardenerConfig specifies whether current Gardener cluster configuration details from provisioner should be included in the response for each runtime
	GardenerConfig bool
	// GlobalAccountIDs parameter filters runtimes by specified global account IDs
	GlobalAccountIDs []string
	// SubAccountIDs parameter filters runtimes by specified subaccount IDs
	SubAccountIDs []string
	// InstanceIDs parameter filters runtimes by specified instance IDs
	InstanceIDs []string
	// RuntimeIDs parameter filters runtimes by specified instance IDs
	RuntimeIDs []string
	// Regions parameter filters runtimes by specified provider regions
	Regions []string
	// Shoots parameter filters runtimes by specified shoot cluster names
	Shoots []string
	// Plans parameter filters runtimes by specified service plans
	Plans []string
	// States parameter filters runtimes by specified runtime states. See type State for possible values
	States []State
	// Expired parameter filters runtimes to show only expired ones.
	Expired bool
	// Events parameter fetches tracing events per instance
	Events string
}

func (rt RuntimeDTO) LastOperation() Operation {
	op := Operation{}

	if rt.Status.Provisioning != nil {
		op = *rt.Status.Provisioning
		op.Type = Provision
	}
	// Take the first cluster upgrade operation, assuming that Data is sorted by CreatedAt DESC.
	if rt.Status.UpgradingCluster != nil && rt.Status.UpgradingCluster.Count > 0 {
		op = rt.Status.UpgradingCluster.Data[0]
		op.Type = UpgradeCluster
	}

	// Take the first unsuspension operation, assuming that Data is sorted by CreatedAt DESC.
	if rt.Status.Unsuspension != nil && rt.Status.Unsuspension.Count > 0 && rt.Status.Unsuspension.Data[0].CreatedAt.After(op.CreatedAt) {
		op = rt.Status.Unsuspension.Data[0]
		op.Type = Unsuspension
	}

	// Take the first suspension operation, assuming that Data is sorted by CreatedAt DESC.
	if rt.Status.Suspension != nil && rt.Status.Suspension.Count > 0 && rt.Status.Suspension.Data[0].CreatedAt.After(op.CreatedAt) {
		op = rt.Status.Suspension.Data[0]
		op.Type = Suspension
	}

	if rt.Status.Deprovisioning != nil && rt.Status.Deprovisioning.CreatedAt.After(op.CreatedAt) {
		op = *rt.Status.Deprovisioning
		op.Type = Deprovision
	}

	// Take the first update operation, assuming that Data is sorted by CreatedAt DESC.
	if rt.Status.Update != nil && rt.Status.Update.Count > 0 && rt.Status.Update.Data[0].CreatedAt.After(op.CreatedAt) {
		op = rt.Status.Update.Data[0]
		op.Type = Update
	}

	return op
}
