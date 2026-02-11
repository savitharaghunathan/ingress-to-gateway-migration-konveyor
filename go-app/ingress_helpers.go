package main

import (
	"fmt"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const nginxAnnotationPrefix = "nginx.ingress.kubernetes.io/"

// IngressConfig holds configuration for building an Ingress resource.
type IngressConfig struct {
	Name        string
	Namespace   string
	Host        string
	Path        string
	ServiceName string
	ServicePort int32
	TLSSecret   string
	Annotations map[string]string
}

// BuildFromConfig creates an Ingress from an IngressConfig.
func BuildFromConfig(cfg IngressConfig) *networkingv1.Ingress {
	nginxClass := "nginx"
	pathType := networkingv1.PathTypePrefix

	annotations := map[string]string{
		"kubernetes.io/ingress.class": "nginx",
	}
	for k, v := range cfg.Annotations {
		annotations[k] = v
	}

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cfg.Name,
			Namespace:   cfg.Namespace,
			Annotations: annotations,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &nginxClass,
			Rules: []networkingv1.IngressRule{
				{
					Host: cfg.Host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     cfg.Path,
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: cfg.ServiceName,
											Port: networkingv1.ServiceBackendPort{
												Number: cfg.ServicePort,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if cfg.TLSSecret != "" {
		ingress.Spec.TLS = []networkingv1.IngressTLS{
			{
				Hosts:      []string{cfg.Host},
				SecretName: cfg.TLSSecret,
			},
		}
	}

	return ingress
}

// ValidateIngress performs basic validation on an Ingress resource.
func ValidateIngress(ingress *networkingv1.Ingress) error {
	if ingress.Name == "" {
		return fmt.Errorf("ingress name cannot be empty")
	}

	if ingress.Spec.IngressClassName == nil {
		if _, ok := ingress.Annotations["kubernetes.io/ingress.class"]; !ok {
			return fmt.Errorf("ingress must specify an IngressClass via spec.ingressClassName or annotation")
		}
	}

	// Check for deprecated controller reference
	if ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName == "nginx" {
		fmt.Println("WARNING: nginx IngressClass detected. ingress-nginx is being retired in March 2026.")
	}

	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service == nil {
				return fmt.Errorf("ingress path %s must specify a backend service", path.Path)
			}
		}
	}

	return nil
}

// HasNginxAnnotation checks if the ingress uses any nginx-specific annotations.
func HasNginxAnnotation(ingress *networkingv1.Ingress) bool {
	for key := range ingress.Annotations {
		if strings.HasPrefix(key, nginxAnnotationPrefix) {
			return true
		}
	}
	return false
}

// GetNginxAnnotations returns all nginx-specific annotations from the ingress.
func GetNginxAnnotations(ingress *networkingv1.Ingress) map[string]string {
	result := make(map[string]string)
	for key, value := range ingress.Annotations {
		if strings.HasPrefix(key, nginxAnnotationPrefix) {
			result[key] = value
		}
	}
	return result
}

// IngressToString provides a human-readable summary of an Ingress resource.
func IngressToString(ingress *networkingv1.Ingress) string {
	summary := fmt.Sprintf("Ingress: %s/%s\n", ingress.Namespace, ingress.Name)
	if ingress.Spec.IngressClassName != nil {
		summary += fmt.Sprintf("  Class: %s\n", *ingress.Spec.IngressClassName)
	}
	for _, rule := range ingress.Spec.Rules {
		summary += fmt.Sprintf("  Host: %s\n", rule.Host)
		if rule.HTTP != nil {
			for _, path := range rule.HTTP.Paths {
				summary += fmt.Sprintf("    Path: %s -> %s:%d\n",
					path.Path,
					path.Backend.Service.Name,
					path.Backend.Service.Port.Number,
				)
			}
		}
	}
	if len(ingress.Spec.TLS) > 0 {
		for _, tls := range ingress.Spec.TLS {
			summary += fmt.Sprintf("  TLS: %v (secret: %s)\n", tls.Hosts, tls.SecretName)
		}
	}
	return summary
}

// MergeIngresses combines rules from multiple Ingress resources into one.
// This is a common pattern when consolidating nginx ingresses.
func MergeIngresses(ingresses []*networkingv1.Ingress) *networkingv1.Ingress {
	if len(ingresses) == 0 {
		return nil
	}

	merged := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ingresses[0].Name + "-merged",
			Namespace:   ingresses[0].Namespace,
			Annotations: make(map[string]string),
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: ingresses[0].Spec.IngressClassName,
		},
	}

	for _, ing := range ingresses {
		merged.Spec.Rules = append(merged.Spec.Rules, ing.Spec.Rules...)
		merged.Spec.TLS = append(merged.Spec.TLS, ing.Spec.TLS...)
		for k, v := range ing.Annotations {
			merged.Annotations[k] = v
		}
	}

	return merged
}
