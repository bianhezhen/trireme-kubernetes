package resolver

import (
	"fmt"

	"go.uber.org/zap"

	"go.aporeto.io/trireme-lib/policy"

	api "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

func clauseEquals(requirement labels.Requirement) []policy.KeyValueOperator {
	return []policy.KeyValueOperator{
		policy.KeyValueOperator{
			Key:      requirement.Key(),
			Operator: policy.Equal,
			Value:    requirement.Values().List(),
		},
	}
}

func clauseNotEquals(requirement labels.Requirement) []policy.KeyValueOperator {
	return []policy.KeyValueOperator{
		policy.KeyValueOperator{
			Key:      requirement.Key(),
			Operator: policy.NotEqual,
			Value:    requirement.Values().List(),
		},
	}
}

func clauseIn(requirement labels.Requirement) []policy.KeyValueOperator {
	return []policy.KeyValueOperator{
		policy.KeyValueOperator{
			Key:      requirement.Key(),
			Operator: policy.Equal,
			Value:    requirement.Values().List(),
		},
	}
}

func clauseNotIn(requirement labels.Requirement) []policy.KeyValueOperator {
	return []policy.KeyValueOperator{
		policy.KeyValueOperator{
			Key:      requirement.Key(),
			Operator: policy.NotEqual,
			Value:    requirement.Values().List(),
		},
	}
}

func clauseExists(requirement labels.Requirement) []policy.KeyValueOperator {
	return []policy.KeyValueOperator{
		policy.KeyValueOperator{
			Key:      requirement.Key(),
			Operator: policy.KeyExists,
			Value:    []string{"*"},
		},
	}
}

func clauseDoesNotExist(requirement labels.Requirement) []policy.KeyValueOperator {
	return []policy.KeyValueOperator{
		policy.KeyValueOperator{
			Key:      requirement.Key(),
			Operator: policy.KeyNotExists,
			Value:    []string{"*"},
		},
	}
}

// portSelector generates all the clauses for the ports
func portSelector(ports []networking.NetworkPolicyPort) []policy.KeyValueOperator {
	// If Port is not defined, then no need for specific traffic matching.
	if ports == nil {
		return []policy.KeyValueOperator{}
	}
	// If Port is defined but no ports are defined into it, then No traffic is matched at all
	if len(ports) == 0 {
		return nil
	}

	portList := []string{}
	for _, port := range ports {
		portList = append(portList, port.Port.String())
	}
	kvo := policy.KeyValueOperator{
		Key:      "$sys:port",
		Operator: policy.Equal,
		Value:    portList,
	}
	return []policy.KeyValueOperator{kvo}

}

func namespaceSelector(namespace string) []policy.KeyValueOperator {
	kvo := policy.KeyValueOperator{
		Key:      UpstreamNamespaceIdentifier,
		Operator: policy.Equal,
		Value:    []string{namespace},
	}
	return []policy.KeyValueOperator{kvo}
}

// podRules generates all the rules for the whole pod.
func podIngressRules(rule *networking.NetworkPolicyIngressRule, namespace string) ([]policy.TagSelector, error) {

	receiverRules := []policy.TagSelector{}
	for _, peer := range rule.From {

		// Individual From. Each From is ORed.
		peerSelector, err := metav1.LabelSelectorAsSelector(peer.PodSelector)
		if err != nil {
			return nil, fmt.Errorf("Error while parsing Peer label selector %s", err)
		}
		peerRequirements, _ := peerSelector.Requirements()

		// Initialize the completeClause with the port matching
		completeClause := []policy.KeyValueOperator{}
		completeClause = append(completeClause, portSelector(rule.Ports)...)

		// Also add the Pod Namespace as a requirement.
		completeClause = append(completeClause, namespaceSelector(namespace)...)

		// Go over each specific requirement and add it as a clause.
		for _, requirement := range peerRequirements {
			// Each requirement is ANDed
			switch requirement.Operator() {
			case selection.Equals:
				requirementClause := clauseEquals(requirement)
				completeClause = append(completeClause, requirementClause...)
			case selection.NotEquals:
				requirementClause := clauseNotEquals(requirement)
				completeClause = append(completeClause, requirementClause...)
			case selection.In:
				requirementClause := clauseIn(requirement)
				completeClause = append(completeClause, requirementClause...)
			case selection.NotIn:
				requirementClause := clauseNotIn(requirement)
				completeClause = append(completeClause, requirementClause...)
			case selection.Exists:
				requirementClause := clauseExists(requirement)
				completeClause = append(completeClause, requirementClause...)
			case selection.DoesNotExist:
				requirementClause := clauseDoesNotExist(requirement)
				completeClause = append(completeClause, requirementClause...)
			}

		}
		selector := policy.TagSelector{
			Clause: completeClause,
			Policy: &policy.FlowPolicy{
				Action: policy.Accept,
			},
		}
		receiverRules = append(receiverRules, selector)
	}

	return receiverRules, nil
}

// podRules generates all the rules for the whole pod.
func podEgressRules(rule *networking.NetworkPolicyEgressRule, namespace string) ([]policy.TagSelector, error) {

	TransmitterRules := []policy.TagSelector{}
	for _, peer := range rule.To {

		// Individual From. Each From is ORed.
		peerSelector, err := metav1.LabelSelectorAsSelector(peer.PodSelector)
		if err != nil {
			return nil, fmt.Errorf("Error while parsing Peer label selector %s", err)
		}
		peerRequirements, _ := peerSelector.Requirements()

		// Initialize the completeClause with the port matching
		completeClause := []policy.KeyValueOperator{}
		completeClause = append(completeClause, portSelector(rule.Ports)...)

		// Also add the Pod Namespace as a requirement.
		completeClause = append(completeClause, namespaceSelector(namespace)...)

		// Go over each specific requirement and add it as a clause.
		for _, requirement := range peerRequirements {
			// Each requirement is ANDed
			switch requirement.Operator() {
			case selection.Equals:
				requirementClause := clauseEquals(requirement)
				completeClause = append(completeClause, requirementClause...)
			case selection.NotEquals:
				requirementClause := clauseNotEquals(requirement)
				completeClause = append(completeClause, requirementClause...)
			case selection.In:
				requirementClause := clauseIn(requirement)
				completeClause = append(completeClause, requirementClause...)
			case selection.NotIn:
				requirementClause := clauseNotIn(requirement)
				completeClause = append(completeClause, requirementClause...)
			case selection.Exists:
				requirementClause := clauseExists(requirement)
				completeClause = append(completeClause, requirementClause...)
			case selection.DoesNotExist:
				requirementClause := clauseDoesNotExist(requirement)
				completeClause = append(completeClause, requirementClause...)
			}

		}
		selector := policy.TagSelector{
			Clause: completeClause,
			Policy: &policy.FlowPolicy{
				Action: policy.Accept,
			},
		}
		TransmitterRules = append(TransmitterRules, selector)
	}

	return TransmitterRules, nil
}

// aclIngressRules generate the IPRules used as ACLs outside of Trireme cluster.
func aclIngressRules(rule networking.NetworkPolicyIngressRule) ([]policy.IPRule, error) {
	aclPolicy := []policy.IPRule{}
	if rule.Ports == nil {
		return nil, fmt.Errorf("Ports entry is nil")
	}

	for _, portEntry := range rule.Ports {
		var proto string
		if *portEntry.Protocol == api.ProtocolUDP {
			proto = "UDP"
		} else if *portEntry.Protocol == api.ProtocolTCP {
			proto = "TCP"
		} else {
			return nil, fmt.Errorf("Unknown ProtocolType")
		}

		ipRuleTCP := policy.IPRule{
			Address:  "0.0.0.0/0",
			Port:     portEntry.Port.String(),
			Protocol: proto,
			Policy: &policy.FlowPolicy{
				Action: policy.Accept,
			},
		}

		ipRuleUDP := policy.IPRule{
			Address:  "0.0.0.0/0",
			Port:     portEntry.Port.String(),
			Protocol: proto,
			Policy: &policy.FlowPolicy{
				Action: policy.Accept,
			},
		}
		aclPolicy = append(aclPolicy, ipRuleTCP, ipRuleUDP)
	}

	return aclPolicy, nil
}

// aclIngressRules generate the IPRules used as ACLs outside of Trireme cluster.
func aclEgressRules(rule networking.NetworkPolicyEgressRule) ([]policy.IPRule, error) {
	aclPolicy := []policy.IPRule{}
	if rule.Ports == nil {
		return nil, fmt.Errorf("Ports entry is nil")
	}

	for _, portEntry := range rule.Ports {
		var proto string
		if *portEntry.Protocol == api.ProtocolUDP {
			proto = "UDP"
		} else if *portEntry.Protocol == api.ProtocolTCP {
			proto = "TCP"
		} else {
			return nil, fmt.Errorf("Unknown ProtocolType")
		}

		ipRuleTCP := policy.IPRule{
			Address:  "0.0.0.0/0",
			Port:     portEntry.Port.String(),
			Protocol: proto,
			Policy: &policy.FlowPolicy{
				Action: policy.Accept,
			},
		}

		ipRuleUDP := policy.IPRule{
			Address:  "0.0.0.0/0",
			Port:     portEntry.Port.String(),
			Protocol: proto,
			Policy: &policy.FlowPolicy{
				Action: policy.Accept,
			},
		}
		aclPolicy = append(aclPolicy, ipRuleTCP, ipRuleUDP)
	}

	return aclPolicy, nil
}

// generateIngressRulesList generates the Trireme receiver rules and ACLs based on a set of Kubernetes IngressRules that apply to a pod.
func generateIngressRulesList(ingressKubeRules *[]networking.NetworkPolicyIngressRule, podNamespace string, allNamespaces *api.NamespaceList) ([]policy.TagSelector, []policy.IPRule, error) {

	// with rules==nil, it means allow all.
	if ingressKubeRules == nil {
		return rulesAndACLsAllowAll()
	}

	// If there are rules, but length ==0 (meaning no actual rules to allow), then it means deny all.
	if len(*ingressKubeRules) == 0 {
		return rulesAndACLsDenyAll()
	}

	receiverRules := []policy.TagSelector{}
	ipRules := []policy.IPRule{}

	// generate IngressRule with tags
	for _, rule := range *ingressKubeRules {

		// From is not set, Only using the Port information.
		if rule.From == nil {
			if rule.Ports == nil {
				// Allow All Ingress  ?
				// Ports also not set: Allow All!
			}

			aclSelectorRules, err := aclIngressRules(rule)
			if err != nil {
				return nil, nil, fmt.Errorf("Error creating pod ACLRules: %s", err)
			}
			ipRules = append(ipRules, aclSelectorRules...)
			continue
		}

		// Not matching any traffic. Go to next rule
		if len(rule.From) == 0 {
			continue
		}

		// Phase1: populate the clauses related to each individual rules.
		podSelectorRules, err := podIngressRules(&rule, podNamespace)
		if err != nil {
			return nil, nil, fmt.Errorf("Error creating pod policyRule: %s", err)
		}
		receiverRules = append(receiverRules, podSelectorRules...)

		// Phase2: populate the clauses related to the namespace rules. (namepace selector...)
		namespaceSelectorRules, err := namespaceIngressRules(&rule, podNamespace, allNamespaces)
		if err != nil {
			return nil, nil, fmt.Errorf("Error creating pod namespaceRule: %s", err)
		}

		receiverRules = append(receiverRules, namespaceSelectorRules...)
	}

	return receiverRules, ipRules, nil
}

func generateEgressRulesList(egressKubeRules *[]networking.NetworkPolicyEgressRule, podNamespace string, allNamespaces *api.NamespaceList) ([]policy.TagSelector, []policy.IPRule, error) {
	// with rules==nil, it means allow all.
	if egressKubeRules == nil {
		return rulesAndACLsAllowAll()
	}

	// If there are rules, but length ==0 (meaning no actual rules to allow), then it means deny all.
	if len(*egressKubeRules) == 0 {
		return rulesAndACLsDenyAll()
	}

	transmitterRules := []policy.TagSelector{}
	ipRules := []policy.IPRule{}

	// generate IngressRule with tags
	for _, rule := range *egressKubeRules {

		// To is not set, Only using the Port information.
		if rule.To == nil {
			if rule.Ports == nil {
				// Ports also not set: Allow All!
				// Allow All Ingress  ?
			}

			aclSelectorRules, err := aclEgressRules(rule)
			if err != nil {
				return nil, nil, fmt.Errorf("Error creating pod ACLRules: %s", err)
			}
			ipRules = append(ipRules, aclSelectorRules...)
			continue
		}

		// Not matching any traffic. Go to next rule
		if len(rule.To) == 0 || len(rule.Ports) == 0 {
			continue
		}

		// Phase1: populate the clauses related to each individual rules.
		podSelectorRules, err := podEgressRules(&rule, podNamespace)
		if err != nil {
			return nil, nil, fmt.Errorf("Error creating pod policyRule: %s", err)
		}
		transmitterRules = append(transmitterRules, podSelectorRules...)

		// Phase2: populate the clauses related to the namespace rules. (namepace selector...)
		namespaceSelectorRules, err := namespaceEgressRules(&rule, podNamespace, allNamespaces)
		if err != nil {
			return nil, nil, fmt.Errorf("Error creating pod namespaceRule: %s", err)
		}

		transmitterRules = append(transmitterRules, namespaceSelectorRules...)
	}

	return transmitterRules, ipRules, nil
}

// namespaceRules generates all the rules associated with the matching of other namespaces
func namespaceIngressRules(rule *networking.NetworkPolicyIngressRule, podNamespace string, allNamespaces *api.NamespaceList) ([]policy.TagSelector, error) {
	receiverRules := []policy.TagSelector{}
	matchedNamespaces := map[string]bool{}
	for _, peer := range rule.From {
		// Individual From. Each From is ORed.
		namespaceSelector, err := metav1.LabelSelectorAsSelector(peer.NamespaceSelector)
		if err != nil {
			return nil, fmt.Errorf("Error while parsing Peer label selector %s", err)
		}
		for _, namespace := range allNamespaces.Items {
			if namespaceSelector.Matches(labels.Set(namespace.GetLabels())) {
				matchedNamespaces[namespace.GetName()] = true
			}
		}
	}

	allowedNamespaces := []string{}
	for namespace := range matchedNamespaces {
		// We don't want to match all of the current namespace.
		if namespace == podNamespace {
			continue
		}
		allowedNamespaces = append(allowedNamespaces, namespace)
	}
	// No need to add the Namespace clause if no namespaces were matched.
	if len(allowedNamespaces) == 0 {
		return nil, nil
	}
	clause := policy.KeyValueOperator{
		Key:      UpstreamNamespaceIdentifier,
		Operator: policy.Equal,
		Value:    allowedNamespaces,
	}

	selector := policy.TagSelector{
		Clause: []policy.KeyValueOperator{clause},
		Policy: &policy.FlowPolicy{
			Action: policy.Accept,
		},
	}

	receiverRules = append(receiverRules, selector)
	return receiverRules, nil
}

// namespaceRules generates all the rules associated with the matching of other namespaces
func namespaceEgressRules(rule *networking.NetworkPolicyEgressRule, podNamespace string, allNamespaces *api.NamespaceList) ([]policy.TagSelector, error) {
	receiverRules := []policy.TagSelector{}
	matchedNamespaces := map[string]bool{}
	for _, peer := range rule.To {
		// Individual From. Each From is ORed.
		namespaceSelector, err := metav1.LabelSelectorAsSelector(peer.NamespaceSelector)
		if err != nil {
			return nil, fmt.Errorf("Error while parsing Peer label selector %s", err)
		}
		for _, namespace := range allNamespaces.Items {
			if namespaceSelector.Matches(labels.Set(namespace.GetLabels())) {
				matchedNamespaces[namespace.GetName()] = true
			}
		}
	}

	allowedNamespaces := []string{}
	for namespace := range matchedNamespaces {
		// We don't want to match all of the current namespace.
		if namespace == podNamespace {
			continue
		}
		allowedNamespaces = append(allowedNamespaces, namespace)
	}
	// No need to add the Namespace clause if no namespaces were matched.
	if len(allowedNamespaces) == 0 {
		return nil, nil
	}
	clause := policy.KeyValueOperator{
		Key:      UpstreamNamespaceIdentifier,
		Operator: policy.Equal,
		Value:    allowedNamespaces,
	}

	selector := policy.TagSelector{
		Clause: []policy.KeyValueOperator{clause},
		Policy: &policy.FlowPolicy{
			Action: policy.Accept,
		},
	}

	receiverRules = append(receiverRules, selector)
	return receiverRules, nil
}

// generatePUPolicy creates a PUPolicy representation
func generatePUPolicy(ingressKubeRules *[]networking.NetworkPolicyIngressRule, egressKubeRules *[]networking.NetworkPolicyEgressRule, podNamespace string, allNamespaces *api.NamespaceList, tags *policy.TagStore, ips policy.ExtendedMap, triremeNets []string) (*policy.PUPolicy, error) {

	ingressRulesList, ingressACLs, err := generateIngressRulesList(ingressKubeRules, podNamespace, allNamespaces)
	if err != nil {
		return nil, fmt.Errorf("Couldn't generate ingress rules: %s", err)
	}

	egressRulesList, egressACLs, err := generateEgressRulesList(egressKubeRules, podNamespace, allNamespaces)
	if err != nil {
		return nil, fmt.Errorf("Couldn't generate egress rules: %s", err)
	}

	excluded := []string{}

	containerPolicy := policy.NewPUPolicy("", policy.Police, egressACLs, ingressACLs, egressRulesList, ingressRulesList, tags, tags, ips, triremeNets, excluded, nil, nil, nil, nil)

	logRules(containerPolicy)
	return containerPolicy, nil
}

func rulesAndACLsAllowAll() ([]policy.TagSelector, []policy.IPRule, error) {
	return rulesAllowAll(), aclsAllowAll(), nil
}

// aclsAllowAll generate the IPRules used as ACLs outside of Trireme cluster.
func aclsAllowAll() []policy.IPRule {
	iPruleTCP := policy.IPRule{
		Address:  "0.0.0.0/0",
		Port:     "0:65535",
		Protocol: "TCP",
		Policy: &policy.FlowPolicy{
			Action: policy.Accept,
		},
	}
	iPruleUDP := policy.IPRule{
		Address:  "0.0.0.0/0",
		Port:     "0:65535",
		Protocol: "UDP",
		Policy: &policy.FlowPolicy{
			Action: policy.Accept,
		},
	}

	return []policy.IPRule{iPruleTCP, iPruleUDP}
}

// rulesAllowAll generate the IPRules used as ACLs outside of Trireme cluster.
func rulesAllowAll() []policy.TagSelector {
	completeClause := []policy.KeyValueOperator{
		policy.KeyValueOperator{
			Key:      UpstreamNamespaceIdentifier,
			Operator: policy.Equal,
			Value:    []string{"*"},
		},
	}
	selector := policy.TagSelector{
		Clause: completeClause,
		Policy: &policy.FlowPolicy{
			Action: policy.Accept,
		},
	}

	return []policy.TagSelector{selector}
}

func rulesAndACLsDenyAll() ([]policy.TagSelector, []policy.IPRule, error) {
	return rulesDenyAll(), aclsDenyAll(), nil
}

// aclsDenyAll generate the IPRules used as ACLs outside of Trireme cluster.
func aclsDenyAll() []policy.IPRule {

	return []policy.IPRule{}
}

// rulesDenyAll generate the IPRules used as ACLs outside of Trireme cluster.
func rulesDenyAll() []policy.TagSelector {

	return []policy.TagSelector{}
}

// allowAllPolicy returns a simple generic policy used in order to not police the PU.
// example: The NS is not networkPolicy activated.
func allowAllPolicy(tags *policy.TagStore, ips policy.ExtendedMap, triremeNets []string) *policy.PUPolicy {
	allowAllACLs := aclsAllowAll()
	receivingRules := rulesAllowAll()
	ingressACLs := allowAllACLs
	egressACLs := allowAllACLs

	excluded := []string{}

	return policy.NewPUPolicy("", policy.Police, ingressACLs, egressACLs, nil, receivingRules, tags, tags, ips, triremeNets, excluded, nil, nil, nil, nil)
}

// logRules logs all the rules currently used. Useful for debugging.
func logRules(containerPolicy *policy.PUPolicy) {
	// INGRESS or RECEIVER or NETWORK Rules and ACLs.
	for i, rule := range containerPolicy.ReceiverRules() {
		for _, clause := range rule.Clause {
			zap.L().Debug("Trireme receiver RULES for POD", zap.Int("i", i), zap.Any("selector", clause))
		}
	}
	for i, acl := range containerPolicy.NetworkACLs() {
		zap.L().Debug("Trireme receiver ACL for POD", zap.Int("i", i), zap.Any("Address", acl.Address), zap.Any("Port", acl.Port), zap.Any("Policy", acl.Policy), zap.Any("Protocol", acl.Protocol))
	}

	// EGRESS or TRANSMITTER or APPLICATION Rules and ACLs.
	for i, rule := range containerPolicy.TransmitterRules() {
		for _, clause := range rule.Clause {
			zap.L().Debug("Trireme transmitter RULES for POD", zap.Int("i", i), zap.Any("selector", clause))
		}
	}
	for i, acl := range containerPolicy.ApplicationACLs() {
		zap.L().Debug("Trireme transmitter ACL for POD", zap.Int("i", i), zap.Any("Address", acl.Address), zap.Any("Port", acl.Port))
	}

	// POD Tags.
	zap.L().Debug("Trireme tags for container X", zap.Any("identity", containerPolicy.Identity()))
}
