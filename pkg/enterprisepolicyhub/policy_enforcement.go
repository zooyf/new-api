package enterprisepolicyhub

import (
	"errors"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

func (a *App) resolvePolicyModelLimits(effective EffectivePolicy) ([]string, bool, error) {
	if effective.AllowedModelsRestricted {
		if len(effective.AllowedModels) == 0 {
			return nil, true, nil
		}
		return effective.AllowedModels, false, nil
	}
	if len(effective.DeniedModels) == 0 {
		return nil, false, nil
	}

	available := make(map[string]struct{})
	var abilities []model.Ability
	if err := a.newAPIDB.Where("enabled = ?", true).Find(&abilities).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	for _, ability := range abilities {
		if name := strings.TrimSpace(ability.Model); name != "" {
			available[name] = struct{}{}
		}
	}
	var channels []model.Channel
	if err := a.newAPIDB.Where("status = ?", common.ChannelStatusEnabled).Find(&channels).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	for _, channel := range channels {
		for _, name := range splitCSV(channel.Models) {
			available[name] = struct{}{}
		}
	}
	if len(available) == 0 {
		return nil, false, errors.New("cannot enforce denied_models because no enabled models are available")
	}
	for _, denied := range effective.DeniedModels {
		delete(available, denied)
	}
	limits := make([]string, 0, len(available))
	for name := range available {
		limits = append(limits, name)
	}
	sort.Strings(limits)
	return limits, len(limits) == 0, nil
}

func (a *App) markKeysPendingForPolicy(policyID int) error {
	ids := make(map[int]struct{})
	var directKeyIDs []int
	if err := a.db.Model(&EnterpriseKey{}).Where("policy_id = ?", policyID).Pluck("id", &directKeyIDs).Error; err != nil {
		return err
	}
	for _, id := range directKeyIDs {
		ids[id] = struct{}{}
	}
	var orgIDs []int
	if err := a.db.Model(&OrgUnit{}).Where("default_policy_id = ?", policyID).Pluck("id", &orgIDs).Error; err != nil {
		return err
	}
	for _, orgID := range orgIDs {
		var closures []OrgUnitClosure
		if err := a.db.Where("ancestor_id = ?", orgID).Find(&closures).Error; err != nil {
			return err
		}
		descendantIDs := []int{orgID}
		if len(closures) > 0 {
			descendantIDs = descendantIDs[:0]
			for _, closure := range closures {
				descendantIDs = append(descendantIDs, closure.DescendantID)
			}
		}
		var inheritedKeyIDs []int
		if err := a.db.Model(&EnterpriseKey{}).Where("org_unit_id IN ?", descendantIDs).Pluck("id", &inheritedKeyIDs).Error; err != nil {
			return err
		}
		for _, id := range inheritedKeyIDs {
			ids[id] = struct{}{}
		}
	}
	return a.markEnterpriseKeyIDsPending(ids)
}

func (a *App) markKeysPendingForOrg(orgID int) error {
	var closures []OrgUnitClosure
	if err := a.db.Where("ancestor_id = ?", orgID).Find(&closures).Error; err != nil {
		return err
	}
	descendantIDs := []int{orgID}
	if len(closures) > 0 {
		descendantIDs = descendantIDs[:0]
		for _, closure := range closures {
			descendantIDs = append(descendantIDs, closure.DescendantID)
		}
	}
	var keyIDs []int
	if err := a.db.Model(&EnterpriseKey{}).Where("org_unit_id IN ?", descendantIDs).Pluck("id", &keyIDs).Error; err != nil {
		return err
	}
	ids := make(map[int]struct{}, len(keyIDs))
	for _, id := range keyIDs {
		ids[id] = struct{}{}
	}
	return a.markEnterpriseKeyIDsPending(ids)
}

func (a *App) markEnterpriseKeyIDsPending(ids map[int]struct{}) error {
	if len(ids) == 0 {
		return nil
	}
	values := make([]int, 0, len(ids))
	for id := range ids {
		values = append(values, id)
	}
	return a.db.Model(&EnterpriseKey{}).Where("id IN ?", values).Update("sync_status", StatusPending).Error
}
