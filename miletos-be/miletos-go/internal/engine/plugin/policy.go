package plugin

import "strings"

type EdgeConstraint struct {
	minimum     uint
	maximum     uint
	hasMaximum  bool
	initialized bool
}

func NewBoundedEdgeConstraint(minimum uint, maximum uint,
) (EdgeConstraint, error) {
	if maximum < minimum {
		return EdgeConstraint{}, newValidationError(
			"maximum", "must be greater than or equal to minimum")
	}
	return EdgeConstraint{
		minimum: minimum, maximum: maximum, hasMaximum: true,
		initialized: true}, nil
}
func NewUnlimitedEdgeConstraint(minimum uint,
) EdgeConstraint {
	return EdgeConstraint{minimum: minimum,
		hasMaximum: false, initialized: true}
}
func NewExactEdgeConstraint(
	count uint) EdgeConstraint {
	return EdgeConstraint{
		minimum: count, maximum: count, hasMaximum: true,
		initialized: true}
}
func (constraint EdgeConstraint) Minimum() uint {
	return constraint.minimum
}
func (constraint EdgeConstraint) Maximum() (
	uint, bool) {
	if !constraint.hasMaximum {
		return 0, false
	}
	return constraint.maximum, true
}
func (constraint EdgeConstraint) IsUnlimited() bool {
	return constraint.initialized &&
		!constraint.hasMaximum
}
func (constraint EdgeConstraint) Allows(actual uint) bool {
	if !constraint.IsValid() {
		return false
	}
	if actual < constraint.minimum {
		return false
	}
	if constraint.hasMaximum &&
		actual > constraint.maximum {
		return false
	}
	return true
}
func (constraint EdgeConstraint) IsValid() bool {
	if !constraint.initialized {
		return false
	}
	if constraint.hasMaximum && constraint.maximum < constraint.minimum {
		return false
	}
	return true
}
func (constraint EdgeConstraint) normalized(
	field string) (EdgeConstraint, error) {
	if !constraint.initialized {
		return EdgeConstraint{}, newValidationError(field, "must be initialized")
	}
	if constraint.hasMaximum {
		if constraint.maximum < constraint.minimum {
			return EdgeConstraint{}, newValidationError(
				field+".maximum", "must be greater than or equal to minimum")
		}
		return NewBoundedEdgeConstraint(
			constraint.minimum, constraint.maximum)
	}
	return NewUnlimitedEdgeConstraint(
		constraint.minimum), nil
}

type DistributionCapability string

const (
	DistributionLocalOnly     DistributionCapability = "LOCAL_ONLY"
	DistributionDistributable DistributionCapability = "DISTRIBUTABLE"
)

func ParseDistributionCapability(value string,
) (DistributionCapability, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch DistributionCapability(normalized) {
	case DistributionLocalOnly:
		return DistributionLocalOnly, nil
	case DistributionDistributable:
		return DistributionDistributable, nil
	default:
		return "", newValidationError("distribution",
			"must be LOCAL_ONLY or DISTRIBUTABLE")
	}
}
func (capability DistributionCapability) String() string {
	return string(capability)
}
func (capability DistributionCapability) IsValid() bool {
	switch capability {
	case DistributionLocalOnly,
		DistributionDistributable:
		return true
	default:
		return false
	}
}
func (capability DistributionCapability) SupportsAsync() bool {
	return capability == DistributionDistributable
}

type OverflowStrategy string

const (
	OverflowStrategyDropOldest OverflowStrategy = "DROP_OLDEST"
)

func ParseOverflowStrategy(value string) (OverflowStrategy, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch OverflowStrategy(normalized) {
	case OverflowStrategyDropOldest:
		return OverflowStrategyDropOldest, nil
	default:
		return "", newValidationError("overflowStrategy", "must be a supported strategy")
	}
}
func (strategy OverflowStrategy) String() string {
	return string(strategy)
}
func (strategy OverflowStrategy) IsValid() bool {
	switch strategy {
	case OverflowStrategyDropOldest:
		return true
	default:
		return false
	}
}

type QueuePolicy struct {
	defaultCapacity  uint
	overflowStrategy OverflowStrategy
	initialized      bool
}

func NewQueuePolicy(defaultCapacity uint, overflowStrategy OverflowStrategy,
) (QueuePolicy, error) {
	if defaultCapacity == 0 {
		return QueuePolicy{}, newValidationError(
			"queuePolicy.defaultCapacity", "must be greater than zero")
	}
	normalizedStrategy, err := ParseOverflowStrategy(
		overflowStrategy.String())
	if err != nil {
		return QueuePolicy{}, newValidationError("queuePolicy.overflowStrategy", "must be a supported strategy")
	}
	return QueuePolicy{defaultCapacity: defaultCapacity, overflowStrategy: normalizedStrategy,
		initialized: true}, nil
}
func (policy QueuePolicy) DefaultCapacity() uint {
	return policy.defaultCapacity
}
func (policy QueuePolicy) OverflowStrategy() OverflowStrategy {
	return policy.overflowStrategy
}
func (policy QueuePolicy) IsValid() bool {
	return policy.initialized && policy.defaultCapacity > 0 &&
		policy.overflowStrategy.IsValid()
}
func (policy QueuePolicy) normalized() (QueuePolicy, error,
) {
	if !policy.initialized {
		return QueuePolicy{}, newValidationError(
			"queuePolicy", "must be initialized")
	}
	return NewQueuePolicy(
		policy.defaultCapacity, policy.overflowStrategy)
}

type CachePolicy struct {
	defaultCapacity  uint
	overflowStrategy OverflowStrategy
	initialized      bool
}

func NewCachePolicy(
	defaultCapacity uint, overflowStrategy OverflowStrategy) (CachePolicy, error) {
	if defaultCapacity == 0 {
		return CachePolicy{}, newValidationError("cachePolicy.defaultCapacity",
			"must be greater than zero")
	}
	normalizedStrategy, err := ParseOverflowStrategy(overflowStrategy.String())
	if err != nil {
		return CachePolicy{}, newValidationError(
			"cachePolicy.overflowStrategy", "must be a supported strategy")
	}
	return CachePolicy{
		defaultCapacity: defaultCapacity, overflowStrategy: normalizedStrategy, initialized: true,
	}, nil
}
func (policy CachePolicy) DefaultCapacity() uint { return policy.defaultCapacity }
func (policy CachePolicy) OverflowStrategy() OverflowStrategy {
	return policy.overflowStrategy
}
func (policy CachePolicy) IsValid() bool {
	return policy.initialized && policy.defaultCapacity > 0 && policy.overflowStrategy.IsValid()
}
func (policy CachePolicy) normalized() (
	CachePolicy, error) {
	if !policy.initialized {
		return CachePolicy{}, newValidationError("cachePolicy",
			"must be initialized")
	}
	return NewCachePolicy(policy.defaultCapacity,
		policy.overflowStrategy)
}
