package client

const (
	K8S_CONFIG_TYPE = "k8sConfig"
)

type K8sConfig struct {
	Resource

	AdmissionControllers []string `json:"admissionControllers,omitempty" yaml:"admission_controllers,omitempty"`

	KubeConfig string `json:"kubeConfig,omitempty" yaml:"kube_config,omitempty"`

	ServiceNetCidr string `json:"serviceNetCidr,omitempty" yaml:"service_net_cidr,omitempty"`
}

type K8sConfigCollection struct {
	Collection
	Data   []K8sConfig `json:"data,omitempty"`
	client *K8sConfigClient
}

type K8sConfigClient struct {
	rancherClient *RancherClient
}

type K8sConfigOperations interface {
	List(opts *ListOpts) (*K8sConfigCollection, error)
	Create(opts *K8sConfig) (*K8sConfig, error)
	Update(existing *K8sConfig, updates interface{}) (*K8sConfig, error)
	ById(id string) (*K8sConfig, error)
	Delete(container *K8sConfig) error
}

func newK8sConfigClient(rancherClient *RancherClient) *K8sConfigClient {
	return &K8sConfigClient{
		rancherClient: rancherClient,
	}
}

func (c *K8sConfigClient) Create(container *K8sConfig) (*K8sConfig, error) {
	resp := &K8sConfig{}
	err := c.rancherClient.doCreate(K8S_CONFIG_TYPE, container, resp)
	return resp, err
}

func (c *K8sConfigClient) Update(existing *K8sConfig, updates interface{}) (*K8sConfig, error) {
	resp := &K8sConfig{}
	err := c.rancherClient.doUpdate(K8S_CONFIG_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *K8sConfigClient) List(opts *ListOpts) (*K8sConfigCollection, error) {
	resp := &K8sConfigCollection{}
	err := c.rancherClient.doList(K8S_CONFIG_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *K8sConfigCollection) Next() (*K8sConfigCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &K8sConfigCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *K8sConfigClient) ById(id string) (*K8sConfig, error) {
	resp := &K8sConfig{}
	err := c.rancherClient.doById(K8S_CONFIG_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *K8sConfigClient) Delete(container *K8sConfig) error {
	return c.rancherClient.doResourceDelete(K8S_CONFIG_TYPE, &container.Resource)
}
