# Trireme deployment on Kubernetes

Trireme-Kubernetes is provided as a bundle. Installation of one or more components depends on your use case. For a bare-minimum cluster deployment with Network Policy enforcement, you only need to deploy the Trireme-Kubernetes DaemonSet.

![Kubernetes-Trireme ecosystem](../docs/architecture.png)

For a Quick and easy way to try Trireme-Kubernetes, follow the instructions to deploy the whole bundle as described on the main README.

## Options for deployment

Trireme-Kubernetes is built as a set of modular Micro-services. Most of them are optional and depending on your use-case you might want to deploy some or all of them.
All of those are deployed into the `kube-system` namespace

Everything is defined as standard Kubernetes YAMLs.

If metrics are collected in your cluster, InfluxDB must be started before the enforcer daemon set for Trireme-Kubernetes gets started.

For all Services (mandatory):
* `config.yaml`: Contains the configuration for everything related to Trireme-Kubernetes. The next section describes the different entries.

Trireme-Kubernetes (mandatory):
* `trireme/enforcer-ds.yaml`: A daemon set used to deploy an instance of the main enforcer binary on each running Kubernetes node.
* `trireme/enforcer-serviceaccount.yaml`: A set of minimal permissions that the Enforcer process should get to access and understand NetworkPolicies on the Kubernetes API.

Trireme-CSR (Optional):
If you don't launch Trireme-CSR you need to provide yourself a PKI for each running instance of Trireme-Kubernetes, or launch the service with the PresharedKey mode.
* `trireme/certificate-crd.yaml`: A definition for a new object type on Kubernetes API called `certificates`. It is defined as a Custom ressource definition and used by Trireme-CSR, the identity service.
* `trireme/csr-rs.yaml`: A replica set that launches the controller for the identity service.
* `trireme/csr-serviceaccount.yaml`: The minimal permissions used by Trireme-CSR in order to read and write on the `certificates` object.

InfluxDB for Time Series metrics(Optional)
All Statistics services are built on top of InfluxDB, as such if you use any of Grafana, Chronograd or Trireme-Graph, you need to also deploy InfluxDB.
* `statistics/influxdb.yaml`: A replica set that launches an instance of InfluxDB that will store all the Flow and Container metrics. Please note that this DB is not HA. You can also provide your own InfluxDB HA Database cluster.

Grafana for metrics (optional)
Grafana is a frontend used for metrics display. It can only be used if you also deployed InfluxDB. 
* `statistics/grafana.yaml`: A replica set for upstream Grafana and a service to access it from outside the cluster (type LoadBalancer).
* `statistics/grafana-setup.yaml`: A job that configures Grafana with a set of tables that display the information into InfluxDB.

Chronograf for metrics (optional)
Chronograd is a frontend used for metric display. It can only be used if you also deployed InfluxDB.
* `statistics/chronograf`: A replica set for upstream chronograf and a service to access it from outside the cluster (type LoadBalancer)

Trireme-Graph for an example of visual graphic display (optional)
Trireme-Graph is a very simple frontend that queries InfluxDB and displays visually all the flows (authorized or rejected) in your cluster between different pods.
* `statistics/graph.yaml`: A replica set for Trireme-Graph and a service of type LoadBalancer to be able to access it from outside the cluster.

## Configuration file

Configuration is filled in through a standard Kubernetes ConfigMap (`config.yaml`). There is a single ConfigMap used for the whole bundle. The config map should be the only place where parameters need to be changed.
Those parameters and associated defaults are:

```
  # Logging: Could be debug, info, warning, error. Info is recommended.
  trireme.log_level: info
  trireme.log_format: human

  # Trireme-Enforcer configuration.
  # Authentication type. Value can be PSK or PKI. (More on the dedicated section)
  trireme.auth_type: PKI

  # Trireme-CSR configuration.
  # defines where to find the CA Certificate and the CA Private Key in case you decide to mount it manually into the pod.
  # Only required if the Trireme-CSR identity service is used.
  trireme.signing_ca_cert: /opt/trireme-csr/configuration/ca-cert.pem
  trireme.signing_ca_cert_key: /opt/trireme-csr/configuration/ca-key.pem

  # Trireme-Statistics configuration.
  # Only required if the metrics and statistics servie is used.
  # InfluxDB endpoint (Collector Interface). By default point to a Cluster Service.
  # Change the DB detail in case you want to use your own InfluxDB Instance.
  trireme.collector_endpoint: http://influxdb:8086
  trireme.collector_user: aporeto
  trireme.collector_password: aporeto
  trireme.collector_db: flowDB
  trireme.collector_insecure_skip_verify: "false"
  # Grafana Configuration.
  # Only required if Grafana initial configuration has to be performed. 
  trireme.grafana_endpoint: http://grafana:3000
  trireme.grafana_user: admin
  trireme.grafana_password: admin
  trireme.grafana_access_type: proxy
```

## PSK vs PKI

Authentication ensures that each identity associated with Kubernetes Pods are not modified or altered before reaching the destination, effectively making a Man In the Middle almost impossible to perform (unlike any traditional enforcement solution relying on IP headers)
Trireme-Kubernetes supports two Authentication modes:

* `PSK` (Or Preshared Key) is the easiest option if the identity service is not used (Trireme-CSR). A preshared password must be generated and set as a Kubernetes secret. The PSK will then be used to sign the identity segment of Pods flows.
* `PKI` is more secure and should be used whenever possible. A unique PKI must be used by each instance of Trireme-Kubernetes. In order to generate and distribute the Keypairs and associated certificates to all Trireme-Kubernetes instances, two options are possible:

- Manual: In this case the user is responsible for generating a unique PKI per trireme-Kubernetes instance and mounting it to each single pod instance.
- Automatic through Trireme-CSR: Use the Trireme-CSR identity service that will issue a Unique KeyPair and certificate upon Trireme-Kubernetes startup.

## Identity service

The identity service is needed only if you chose to use a `PKI` deployment model. In this case, the identity service will automatically generate a keypair for each Trireme-Kubernetes instance. The Keypair and associated certificate will be generated using the CA provided in the previous section.

There is also the option to distribute the identity manually without the help of the identity-service. In which case the user is responsible for mounting the certificate and keypair on each pod instance (on each node). This process will be documented shortly.

All the code behind the identity service can be found on the [Trireme-CSR](https://github.com/aporeto-inc/trireme-csr) repository

## Statistics service

The statistics service bundle is an optional service that is based on a basic InfluxDB Metric database. Each flow and container event going through the cluster is recorded as a Time series event.

Multiple tools can be used to extract analytics out of those events. Trireme ships with three very basic options. For more complex use-cases it is very easy to use the InfluxDB Interface directly.

In order to use the Statistics service, an InfluxDB service must be available. For any serious usage, it is recommended to deploy a redundant InfluxDB endpoint.
A simple standalone instance of InfluxDB can be instantiated by using the `statistics/influxdb.yaml` definition.

The three options for displaying metrics and analytics are:

* Chronograf: The official display tool for InfluxDB. Service and pod definitions can be displayed with a simple Database Query
* Grafana: Grafana is preconfigured to connect to InfluxDB and display Container and Flow events in a table.
* Trireme-graph: Connects to InfluxDB and generates a graph that represents interaction between pods. The graph can be customized to show only links and pods that have events in a specific namespace and timefrane
