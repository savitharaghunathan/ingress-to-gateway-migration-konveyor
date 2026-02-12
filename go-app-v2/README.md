# orcapod-ingress-provisioner

A Go CLI tool that provisions standardized Ingress resources for tenant applications using nginx-ingress. Each tenant gets a consistent set of Ingress resources with the company's default security headers and TLS configuration.

This tool needs to be migrated from nginx-ingress to Gateway API before the March 2026 retirement deadline.

## Structure

Single file: `main.go` — provisions ingresses for the storefront (web frontend + API backend), with CRUD operations, ingress builder, IngressClass management, security headers, HSTS, and validation.

## Nginx-Ingress Patterns Used

All patterns below have direct Gateway API equivalents:

- **Core types**: `Ingress`, `IngressSpec`, `IngressRule`, `IngressTLS`, `IngressClass`, `IngressClassSpec`, `IngressBackend`, `IngressServiceBackend`, `ServiceBackendPort`, `HTTPIngressPath`, `HTTPIngressRuleValue`, `PathTypePrefix`
- **Client-go API**: `NetworkingV1().Ingresses()`, `NetworkingV1().IngressClasses()`
- **Annotations**: ssl-redirect, configuration-snippet (response headers), proxy-read-timeout, proxy-send-timeout, hsts/hsts-max-age/hsts-include-subdomains, kubernetes.io/ingress.class, k8s.io/ingress-nginx controller
