#XDS operator

An operator SDK based XDS management server. There are two main parts combined, a Kubernetes operator and an XDS server. The K8s operator is in charge to detecting new CRs, read their properties, update them if it's needed and so on... XDS server's duty is to handle connected XDS clients (in our use-case: Envoy instances). By 'handling' configuring is meant. The CRs mentioned earlier are configurations for the connected XDS clients. 

There are still some things to do, but current version is capable to configure the clients with `strictdns` service discovery. `EDS` is in progress.
