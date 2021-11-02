#XDS operator

An operator SDK based XDS management server. There are two main parts combined, a Kubernetes operator and an XDS server. The K8s operator is in charge to detecting new CRs, read their properties, update them if it's needed and so on... XDS server's duty is to handle connected XDS clients (in our use-case: Envoy instances). By 'handling' configuring is meant. The CRs mentioned earlier are configurations for the connected XDS clients. 

There are still some things to do, but current version is capable to configure the clients with `strictdns` service discovery. `EDS` is in progress.

### Building and deploying the operator

Currently, the easiest method to use the operator is to build the image locally `make docker-build`
and deploy it into your Kubernetes cluster using `make deploy`. To do this you must have 
[Kustomize](https://kustomize.io/) installed in your PATH env.


### Configuring

Although, Envoy's API is complex and huge we tried to keep our custom API as simple as it can be.
VirtualService (vsvc) CRD is being used to create CRs. 
These CRs are created (added to the cluster via the K8s API) by other pieces of software typically
running in other container(s). 

#### Permission

That container must have permission to create these vsvc CRs. To do that create a ClusterRole with a rule like the snippet below.
```
- apiGroups: ["servicemesh.l7mp.io"]
  resources: ["virtualservices"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

#### VirtualServices

To create these custom objects you can use any languages' k8s CustomObjectApi package. 
The base of every vsvc should look like this:
```
resource = {
    'apiVersion': 'servicemesh.l7mp.io/v1',
    'kind': 'VirtualService',
    'metadata': {
        'name': <UNIQUE_NAME>
    },
    'spec': {
        'selector': {
            <POD's LABEL key>: <POD's LABEL VALUE>
        },
        'listeners': []
    }
}
```
Listeners field takes a list where all items are describing a full pipe from the listening port to the upstream endpoint.
Please consider to give unique name to every single listener and cluster even through multiple CRs to prevent name collision problems. (NOT tested)
```
listeners:
- name: <UNIQUE_NAME> #ingress-callid-l
  udp:
    port: 2001
    cluster:
      name: <UNIQUE_NAME> #ingress-callid-c
      service_discovery: strictdns
      health_check:
        interval: 500 #500 ms at least
        protocol: TCP #TCP only
      hash_key: callid
      endpoints:
        - name: <UNIQUE_NAME> #endpoint-name-1
          host:
            selector:
              app: worker
          port: 1234
          health_check_port: 1233
        - name: <UNIQUE_NAME> #endpoint-name-2
          host:
            address: tcp-echo.default.svc # can be an IP address as well
          port: 1234
          health_check_port: 1233
```
- Every listener's IP address is `0.0.0.0` so it is enough to care only about the port and not the IP address.
- `Port` is where the listener will listen to new datagrams.
- `Cluster` is holding a single or multiple upstream endpoints.
- `Name` is the name of the cluster. UNIQUE
- `service_discovery` `strictdns` only for now. To support scalability `EDS` is in progress
- `healt_check` if you want to health check the endpoints please set this field like above.
- `hash_key` if there are multiple worker pods that may handle your streams please set this field.
Best practice is to set it to a unique ID like a call ID if there's one but the string does not really matter.
- `endpoints` is a list of upstream hosts or endpoints. 
- Under `host` you can define a pod label selector or a domain name or a direct IP address.
- Endpoint's `port` is the port where the proxy should forward the data.
- `health_check_port` is the port on the upstream host where the TCP connection of the health checking process should be accepted


