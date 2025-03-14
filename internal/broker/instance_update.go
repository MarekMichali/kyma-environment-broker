package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/kyma-project/kyma-environment-broker/internal/regionssupportingmachine"

	pkg "github.com/kyma-project/kyma-environment-broker/common/runtime"
	"github.com/kyma-project/kyma-environment-broker/internal/assuredworkloads"

	"github.com/kyma-incubator/compass/components/director/pkg/jsonschema"
	"github.com/kyma-project/kyma-environment-broker/internal/euaccess"
	"github.com/kyma-project/kyma-environment-broker/internal/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	"github.com/pivotal-cf/brokerapi/v12/domain"
	"github.com/pivotal-cf/brokerapi/v12/domain/apiresponses"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/kyma-project/kyma-environment-broker/internal"
	"github.com/kyma-project/kyma-environment-broker/internal/dashboard"
	"github.com/kyma-project/kyma-environment-broker/internal/ptr"
	"github.com/kyma-project/kyma-environment-broker/internal/storage"
	"github.com/kyma-project/kyma-environment-broker/internal/storage/dberr"
)

type ContextUpdateHandler interface {
	Handle(instance *internal.Instance, newCtx internal.ERSContext) (bool, error)
}

type UpdateEndpoint struct {
	config Config
	log    *slog.Logger

	instanceStorage                          storage.Instances
	runtimeStates                            storage.RuntimeStates
	contextUpdateHandler                     ContextUpdateHandler
	brokerURL                                string
	processingEnabled                        bool
	subaccountMovementEnabled                bool
	updateCustomResourcesLabelsOnAccountMove bool

	operationStorage storage.Operations

	updatingQueue Queue

	plansConfig  PlansConfig
	planDefaults PlanDefaults

	dashboardConfig dashboard.Config
	kcBuilder       kubeconfig.KcBuilder

	convergedCloudRegionsProvider ConvergedCloudRegionProvider

	regionsSupportingMachine map[string][]string

	kcpClient client.Client
}

func NewUpdate(cfg Config,
	instanceStorage storage.Instances,
	runtimeStates storage.RuntimeStates,
	operationStorage storage.Operations,
	ctxUpdateHandler ContextUpdateHandler,
	processingEnabled bool,
	subaccountMovementEnabled bool,
	updateCustomResourcesLabelsOnAccountMove bool,
	queue Queue,
	plansConfig PlansConfig,
	planDefaults PlanDefaults,
	log *slog.Logger,
	dashboardConfig dashboard.Config,
	kcBuilder kubeconfig.KcBuilder,
	convergedCloudRegionsProvider ConvergedCloudRegionProvider,
	kcpClient client.Client,
	regionsSupportingMachine map[string][]string,
) *UpdateEndpoint {
	return &UpdateEndpoint{
		config:                                   cfg,
		log:                                      log.With("service", "UpdateEndpoint"),
		instanceStorage:                          instanceStorage,
		runtimeStates:                            runtimeStates,
		operationStorage:                         operationStorage,
		contextUpdateHandler:                     ctxUpdateHandler,
		processingEnabled:                        processingEnabled,
		subaccountMovementEnabled:                subaccountMovementEnabled,
		updateCustomResourcesLabelsOnAccountMove: updateCustomResourcesLabelsOnAccountMove,
		updatingQueue:                            queue,
		plansConfig:                              plansConfig,
		planDefaults:                             planDefaults,
		dashboardConfig:                          dashboardConfig,
		kcBuilder:                                kcBuilder,
		convergedCloudRegionsProvider:            convergedCloudRegionsProvider,
		kcpClient:                                kcpClient,
		regionsSupportingMachine:                 regionsSupportingMachine,
	}
}

// Update modifies an existing service instance
//
//	PATCH /v2/service_instances/{instance_id}
func (b *UpdateEndpoint) Update(_ context.Context, instanceID string, details domain.UpdateDetails, asyncAllowed bool) (domain.UpdateServiceSpec, error) {
	logger := b.log.With("instanceID", instanceID)
	logger.Info(fmt.Sprintf("Updating instanceID: %s", instanceID))
	logger.Info(fmt.Sprintf("Updating asyncAllowed: %v", asyncAllowed))
	logger.Info(fmt.Sprintf("Parameters: '%s'", string(details.RawParameters)))
	instance, err := b.instanceStorage.GetByID(instanceID)
	if err != nil && dberr.IsNotFound(err) {
		logger.Error(fmt.Sprintf("unable to get instance: %s", err.Error()))
		return domain.UpdateServiceSpec{}, apiresponses.NewFailureResponse(err, http.StatusNotFound, fmt.Sprintf("could not execute update for instanceID %s", instanceID))
	} else if err != nil {
		logger.Error(fmt.Sprintf("unable to get instance: %s", err.Error()))
		return domain.UpdateServiceSpec{}, fmt.Errorf("unable to get instance")
	}
	logger.Info(fmt.Sprintf("Plan ID/Name: %s/%s", instance.ServicePlanID, PlanNamesMapping[instance.ServicePlanID]))
	var ersContext internal.ERSContext
	err = json.Unmarshal(details.RawContext, &ersContext)
	if err != nil {
		logger.Error(fmt.Sprintf("unable to decode context: %s", err.Error()))
		return domain.UpdateServiceSpec{}, fmt.Errorf("unable to unmarshal context")
	}
	logger.Info(fmt.Sprintf("Global account ID: %s active: %s", instance.GlobalAccountID, ptr.BoolAsString(ersContext.Active)))
	logger.Info(fmt.Sprintf("Received context: %s", marshallRawContext(hideSensitiveDataFromRawContext(details.RawContext))))
	// validation of incoming input
	if err := b.validateWithJsonSchemaValidator(details, instance); err != nil {
		return domain.UpdateServiceSpec{}, apiresponses.NewFailureResponse(err, http.StatusBadRequest, "validation failed")
	}

	if instance.IsExpired() {
		if b.config.AllowUpdateExpiredInstanceWithContext && ersContext.GlobalAccountID != "" {
			return domain.UpdateServiceSpec{}, nil
		}
		return domain.UpdateServiceSpec{}, apiresponses.NewFailureResponse(fmt.Errorf("cannot update an expired instance"), http.StatusBadRequest, "")
	}
	lastProvisioningOperation, err := b.operationStorage.GetProvisioningOperationByInstanceID(instance.InstanceID)
	if err != nil {
		logger.Error(fmt.Sprintf("cannot fetch provisioning lastProvisioningOperation for instance with ID: %s : %s", instance.InstanceID, err.Error()))
		return domain.UpdateServiceSpec{}, fmt.Errorf("unable to process the update")
	}
	if lastProvisioningOperation.State == domain.Failed {
		return domain.UpdateServiceSpec{}, apiresponses.NewFailureResponse(fmt.Errorf("Unable to process an update of a failed instance"), http.StatusUnprocessableEntity, "")
	}

	lastDeprovisioningOperation, err := b.operationStorage.GetDeprovisioningOperationByInstanceID(instance.InstanceID)
	if err != nil && !dberr.IsNotFound(err) {
		logger.Error(fmt.Sprintf("cannot fetch deprovisioning for instance with ID: %s : %s", instance.InstanceID, err.Error()))
		return domain.UpdateServiceSpec{}, fmt.Errorf("unable to process the update")
	}
	if err == nil {
		if !lastDeprovisioningOperation.Temporary {
			// it is not a suspension, but real deprovisioning
			logger.Warn(fmt.Sprintf("Cannot process update, the instance has started deprovisioning process (operationID=%s)", lastDeprovisioningOperation.Operation.ID))
			return domain.UpdateServiceSpec{}, apiresponses.NewFailureResponse(fmt.Errorf("Unable to process an update of a deprovisioned instance"), http.StatusUnprocessableEntity, "")
		}
	}

	dashboardURL := instance.DashboardURL
	if b.dashboardConfig.LandscapeURL != "" {
		dashboardURL = fmt.Sprintf("%s/?kubeconfigID=%s", b.dashboardConfig.LandscapeURL, instanceID)
		instance.DashboardURL = dashboardURL
	}

	if b.processingEnabled {
		instance, suspendStatusChange, err := b.processContext(instance, details, lastProvisioningOperation, logger)
		if err != nil {
			return domain.UpdateServiceSpec{}, err
		}

		// NOTE: KEB currently can't process update parameters in one call along with context update
		// this block makes it that KEB ignores any parameters updates if context update changed suspension state
		if !suspendStatusChange && !instance.IsExpired() {
			return b.processUpdateParameters(instance, details, lastProvisioningOperation, asyncAllowed, ersContext, logger)
		}
	}
	return domain.UpdateServiceSpec{
		IsAsync:       false,
		DashboardURL:  dashboardURL,
		OperationData: "",
		Metadata: domain.InstanceMetadata{
			Labels: ResponseLabels(*lastProvisioningOperation, *instance, b.config.URL, b.config.EnableKubeconfigURLLabel, b.kcBuilder),
		},
	}, nil
}

func (b *UpdateEndpoint) validateWithJsonSchemaValidator(details domain.UpdateDetails, instance *internal.Instance) error {
	if len(details.RawParameters) > 0 {
		planValidator, err := b.getJsonSchemaValidator(instance.Provider, instance.ServicePlanID, instance.Parameters.PlatformRegion)
		if err != nil {
			return fmt.Errorf("while creating plan validator: %w", err)
		}
		result, err := planValidator.ValidateString(string(details.RawParameters))
		if err != nil {
			return fmt.Errorf("while executing JSON schema validator: %w", err)
		}
		if !result.Valid {
			return fmt.Errorf("while validating update parameters: %w", result.Error)
		}
	}
	return nil
}

func shouldUpdate(instance *internal.Instance, details domain.UpdateDetails, ersContext internal.ERSContext) bool {
	if len(details.RawParameters) != 0 {
		return true
	}
	return ersContext.ERSUpdate()
}

func (b *UpdateEndpoint) processUpdateParameters(instance *internal.Instance, details domain.UpdateDetails, lastProvisioningOperation *internal.ProvisioningOperation, asyncAllowed bool, ersContext internal.ERSContext, logger *slog.Logger) (domain.UpdateServiceSpec, error) {
	if !shouldUpdate(instance, details, ersContext) {
		logger.Debug("Parameters not provided, skipping processing update parameters")
		return domain.UpdateServiceSpec{
			IsAsync:       false,
			DashboardURL:  instance.DashboardURL,
			OperationData: "",
			Metadata: domain.InstanceMetadata{
				Labels: ResponseLabels(*lastProvisioningOperation, *instance, b.config.URL, b.config.EnableKubeconfigURLLabel, b.kcBuilder),
			},
		}, nil
	}
	// asyncAllowed needed, see https://github.com/openservicebrokerapi/servicebroker/blob/v2.16/spec.md#updating-a-service-instance
	if !asyncAllowed {
		return domain.UpdateServiceSpec{}, apiresponses.ErrAsyncRequired
	}
	var params internal.UpdatingParametersDTO
	if len(details.RawParameters) != 0 {
		err := json.Unmarshal(details.RawParameters, &params)
		if err != nil {
			logger.Error(fmt.Sprintf("unable to unmarshal parameters: %s", err.Error()))
			return domain.UpdateServiceSpec{}, fmt.Errorf("unable to unmarshal parameters")
		}
		logger.Debug(fmt.Sprintf("Updating with params: %+v", params))
	}

	if !regionssupportingmachine.IsSupported(b.regionsSupportingMachine, valueOfPtr(instance.Parameters.Parameters.Region), valueOfPtr(params.MachineType)) {
		message := fmt.Sprintf(
			"In the region %s, the machine type %s is not available, it is supported in the %v",
			valueOfPtr(instance.Parameters.Parameters.Region),
			valueOfPtr(params.MachineType),
			strings.Join(regionssupportingmachine.SupportedRegions(b.regionsSupportingMachine, valueOfPtr(params.MachineType)), ", "),
		)
		return domain.UpdateServiceSpec{}, apiresponses.NewFailureResponse(fmt.Errorf(message), http.StatusBadRequest, message)
	}

	if params.OIDC.IsProvided() {
		if err := params.OIDC.Validate(); err != nil {
			logger.Error(fmt.Sprintf("invalid OIDC parameters: %s", err.Error()))
			return domain.UpdateServiceSpec{}, apiresponses.NewFailureResponse(err, http.StatusUnprocessableEntity, err.Error())
		}
	}

	operationID := uuid.New().String()
	logger = logger.With("operationID", operationID)

	logger.Debug(fmt.Sprintf("creating update operation %v", params))
	operation := internal.NewUpdateOperation(operationID, instance, params)
	planID := instance.Parameters.PlanID
	if len(details.PlanID) != 0 {
		planID = details.PlanID
	}
	defaults, err := b.planDefaults(planID, instance.Provider, &instance.Provider)
	if err != nil {
		logger.Error(fmt.Sprintf("unable to obtain plan defaults: %s", err.Error()))
		return domain.UpdateServiceSpec{}, fmt.Errorf("unable to obtain plan defaults")
	}
	var autoscalerMin, autoscalerMax int
	if defaults.GardenerConfig != nil {
		p := defaults.GardenerConfig
		autoscalerMin, autoscalerMax = p.AutoScalerMin, p.AutoScalerMax
	}
	if err := operation.ProvisioningParameters.Parameters.AutoScalerParameters.Validate(autoscalerMin, autoscalerMax); err != nil {
		logger.Error(fmt.Sprintf("invalid autoscaler parameters: %s", err.Error()))
		return domain.UpdateServiceSpec{}, apiresponses.NewFailureResponse(err, http.StatusBadRequest, err.Error())
	}

	if params.AdditionalWorkerNodePools != nil {
		if !supportsAdditionalWorkerNodePools(details.PlanID) {
			message := fmt.Sprintf("additional worker node pools are not supported for plan ID: %s", details.PlanID)
			return domain.UpdateServiceSpec{}, apiresponses.NewFailureResponse(fmt.Errorf(message), http.StatusBadRequest, message)
		}
		if !AreNamesUnique(params.AdditionalWorkerNodePools) {
			message := "names of additional worker node pools must be unique"
			return domain.UpdateServiceSpec{}, apiresponses.NewFailureResponse(fmt.Errorf(message), http.StatusBadRequest, message)
		}
		for _, additionalWorkerNodePool := range params.AdditionalWorkerNodePools {
			if err := additionalWorkerNodePool.Validate(); err != nil {
				return domain.UpdateServiceSpec{}, apiresponses.NewFailureResponse(err, http.StatusBadRequest, err.Error())
			}
			if err := additionalWorkerNodePool.ValidateHAZonesUnchanged(instance.Parameters.Parameters.AdditionalWorkerNodePools); err != nil {
				return domain.UpdateServiceSpec{}, apiresponses.NewFailureResponse(err, http.StatusBadRequest, err.Error())
			}
		}
		if isExternalCustomer(ersContext) {
			if err := checkGPUMachinesUsage(params.AdditionalWorkerNodePools); err != nil {
				return domain.UpdateServiceSpec{}, apiresponses.NewFailureResponse(err, http.StatusBadRequest, err.Error())
			}
		}
		if err := checkUnsupportedMachines(b.regionsSupportingMachine, valueOfPtr(instance.Parameters.Parameters.Region), params.AdditionalWorkerNodePools); err != nil {
			return domain.UpdateServiceSpec{}, apiresponses.NewFailureResponse(err, http.StatusBadRequest, err.Error())
		}
	}

	err = b.operationStorage.InsertOperation(operation)
	if err != nil {
		return domain.UpdateServiceSpec{}, err
	}

	var updateStorage []string
	if params.OIDC.IsProvided() {
		instance.Parameters.Parameters.OIDC = params.OIDC
		updateStorage = append(updateStorage, "OIDC")
	}

	if len(params.RuntimeAdministrators) != 0 {
		newAdministrators := make([]string, 0, len(params.RuntimeAdministrators))
		newAdministrators = append(newAdministrators, params.RuntimeAdministrators...)
		instance.Parameters.Parameters.RuntimeAdministrators = newAdministrators
		updateStorage = append(updateStorage, "Runtime Administrators")
	}

	if params.UpdateAutoScaler(&instance.Parameters.Parameters) {
		updateStorage = append(updateStorage, "Auto Scaler parameters")
	}
	if params.MachineType != nil && *params.MachineType != "" {
		instance.Parameters.Parameters.MachineType = params.MachineType
	}

	if supportsAdditionalWorkerNodePools(details.PlanID) && params.AdditionalWorkerNodePools != nil {
		newAdditionalWorkerNodePools := make([]pkg.AdditionalWorkerNodePool, 0, len(params.AdditionalWorkerNodePools))
		newAdditionalWorkerNodePools = append(newAdditionalWorkerNodePools, params.AdditionalWorkerNodePools...)
		instance.Parameters.Parameters.AdditionalWorkerNodePools = newAdditionalWorkerNodePools
		updateStorage = append(updateStorage, "Additional Worker Node Pools")
	}

	if len(updateStorage) > 0 {
		if err := wait.PollUntilContextTimeout(context.Background(), 500*time.Millisecond, 2*time.Second, true, func(ctx context.Context) (bool, error) {
			instance, err = b.instanceStorage.Update(*instance)
			if err != nil {
				params := strings.Join(updateStorage, ", ")
				logger.Warn(fmt.Sprintf("unable to update instance with new %v (%s), retrying", params, err.Error()))
				return false, nil
			}
			return true, nil
		}); err != nil {
			response := apiresponses.NewFailureResponse(fmt.Errorf("Update operation failed"), http.StatusInternalServerError, err.Error())
			return domain.UpdateServiceSpec{}, response
		}
	}
	logger.Debug("Adding update operation to the processing queue")
	b.updatingQueue.Add(operationID)

	return domain.UpdateServiceSpec{
		IsAsync:       true,
		DashboardURL:  instance.DashboardURL,
		OperationData: operation.ID,
		Metadata: domain.InstanceMetadata{
			Labels: ResponseLabels(*lastProvisioningOperation, *instance, b.config.URL, b.config.EnableKubeconfigURLLabel, b.kcBuilder),
		},
	}, nil
}

func (b *UpdateEndpoint) processContext(instance *internal.Instance, details domain.UpdateDetails, lastProvisioningOperation *internal.ProvisioningOperation, logger *slog.Logger) (*internal.Instance, bool, error) {
	var ersContext internal.ERSContext
	err := json.Unmarshal(details.RawContext, &ersContext)
	if err != nil {
		logger.Error(fmt.Sprintf("unable to decode context: %s", err.Error()))
		return nil, false, fmt.Errorf("unable to unmarshal context")
	}
	logger.Info(fmt.Sprintf("Global account ID: %s active: %s", instance.GlobalAccountID, ptr.BoolAsString(ersContext.Active)))

	lastOp, err := b.operationStorage.GetLastOperation(instance.InstanceID)
	if err != nil {
		logger.Error(fmt.Sprintf("unable to get last operation: %s", err.Error()))
		return nil, false, fmt.Errorf("failed to process ERS context")
	}

	// todo: remove the code below when we are sure the ERSContext contains required values.
	// This code is done because the PATCH request contains only some of fields and that requests made the ERS context empty in the past.
	existingSMOperatorCredentials := instance.Parameters.ErsContext.SMOperatorCredentials
	instance.Parameters.ErsContext = lastProvisioningOperation.ProvisioningParameters.ErsContext
	// but do not change existing SM operator credentials
	instance.Parameters.ErsContext.SMOperatorCredentials = existingSMOperatorCredentials
	instance.Parameters.ErsContext.Active, err = b.extractActiveValue(instance.InstanceID, *lastProvisioningOperation)
	if err != nil {
		return nil, false, fmt.Errorf("unable to process the update")
	}
	instance.Parameters.ErsContext = internal.InheritMissingERSContext(instance.Parameters.ErsContext, lastOp.ProvisioningParameters.ErsContext)
	instance.Parameters.ErsContext = internal.UpdateInstanceERSContext(instance.Parameters.ErsContext, ersContext)

	changed, err := b.contextUpdateHandler.Handle(instance, ersContext)
	if err != nil {
		logger.Error(fmt.Sprintf("processing context updated failed: %s", err.Error()))
		return nil, changed, fmt.Errorf("unable to process the update")
	}

	//  copy the Active flag if set
	if ersContext.Active != nil {
		instance.Parameters.ErsContext.Active = ersContext.Active
	}

	needUpdateCustomResources := false
	if b.subaccountMovementEnabled && (instance.GlobalAccountID != ersContext.GlobalAccountID && ersContext.GlobalAccountID != "") {
		if instance.SubscriptionGlobalAccountID == "" {
			instance.SubscriptionGlobalAccountID = instance.GlobalAccountID
		}
		instance.GlobalAccountID = ersContext.GlobalAccountID
		needUpdateCustomResources = true
		logger.Info(fmt.Sprintf("Global account ID changed to: %s. need update labels", instance.GlobalAccountID))
	}

	newInstance, err := b.instanceStorage.Update(*instance)
	if err != nil {
		logger.Error(fmt.Sprintf("instance updated failed: %s", err.Error()))
		return nil, changed, fmt.Errorf("unable to process the update")
	} else if b.updateCustomResourcesLabelsOnAccountMove && needUpdateCustomResources {
		logger.Info("updating labels on related CRs")
		// update labels on related CRs, but only if account movement was successfully persisted and kept in database
		labeler := NewLabeler(b.kcpClient)
		err = labeler.UpdateLabels(newInstance.RuntimeID, newInstance.GlobalAccountID)
		if err != nil {
			logger.Error(fmt.Sprintf("unable to update global account label on CRs while doing account move: %s", err.Error()))
			response := apiresponses.NewFailureResponse(fmt.Errorf("Update CRs label failed"), http.StatusInternalServerError, err.Error())
			return newInstance, changed, response
		}
		logger.Info("labels updated")
	}

	return newInstance, changed, nil
}

func (b *UpdateEndpoint) extractActiveValue(id string, provisioning internal.ProvisioningOperation) (*bool, error) {
	deprovisioning, dErr := b.operationStorage.GetDeprovisioningOperationByInstanceID(id)
	if dErr != nil && !dberr.IsNotFound(dErr) {
		b.log.Error(fmt.Sprintf("Unable to get deprovisioning operation for the instance %s to check the active flag: %s", id, dErr.Error()))
		return nil, dErr
	}
	// there was no any deprovisioning in the past (any suspension)
	if deprovisioning == nil {
		return ptr.Bool(true), nil
	}

	return ptr.Bool(deprovisioning.CreatedAt.Before(provisioning.CreatedAt)), nil
}

func (b *UpdateEndpoint) getJsonSchemaValidator(provider pkg.CloudProvider, planID string, platformRegion string) (JSONSchemaValidator, error) {
	// shootAndSeedSameRegion is never enabled for update
	b.log.Info(fmt.Sprintf("region is: %s", platformRegion))
	plans := Plans(b.plansConfig, provider, nil, b.config.IncludeAdditionalParamsInSchema, euaccess.IsEURestrictedAccess(platformRegion), b.config.UseSmallerMachineTypes, false, b.convergedCloudRegionsProvider.GetRegions(platformRegion), assuredworkloads.IsKSA(platformRegion), b.config.UseAdditionalOIDCSchema)
	plan := plans[planID]
	schema := string(Marshal(plan.Schemas.Instance.Update.Parameters))

	return jsonschema.NewValidatorFromStringSchema(schema)
}
