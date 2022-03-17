package eks

import (
	"encoding/json"
	"fmt"
	"github.com/pulumi/pulumi-aws/sdk/v4/go/aws/eks"
	"github.com/pulumi/pulumi-aws/sdk/v4/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"strings"
)

type EksConfigInput struct {
	// required user input
	K8sVersion       string                 `json:"k8s-version"`
	NodeGroupVersion string                 `json:"nodegroup-version"`
	NodeGroupConfig  []NodeGroupConfigInput `json:"node-groups"`

	// optional user input
	KubeConfigAssumeRoleArn string `json:"kubeconfig-assume-role-arn"`
	KubeConfigAwsProfile    string `json:"kubeconfig-aws-profile"`

	// optional cluster autoscaler IRSA configuration
	ClusterAutoscalerServiceAccount string `json:"cluster-autoscaler-serviceaccount"`
	ClusterAutoscalerNamespace      string `json:"cluster-autoscaler-namespace"`

	// input from vpc module
	SubnetIDs []pulumi.StringOutput
}

type NodeGroupConfigInput struct {
	Name          string   `json:"name"`
	DesiredSize   int      `json:"desired-size"`
	MaxSize       int      `json:"max-size"`
	MinSize       int      `json:"min-size"`
	InstanceTypes []string `json:"instance-types"`
}

// https://github.com/hashicorp/terraform-provider-aws/issues/10104#issuecomment-545264374
// TODO: generate this instead
var awsRootCAThumbprint string = "9e99a48a9960b14926bb7f3b02e22da2b0ab7280"

func CreateEksCluster(ctx *pulumi.Context, eksConfig EksConfigInput) error {
	clusterName := ctx.Stack()

	// allow nodegroups to have a different version for upgrade process,
	// default to cluster version if not specified
	if eksConfig.NodeGroupVersion != "" {
		eksConfig.NodeGroupVersion = eksConfig.K8sVersion
	}

	// set default values of config input, if they aren't supplied
	clusterAutoscalerServiceAccount := "cluster-autoscaler"
	if eksConfig.ClusterAutoscalerServiceAccount != "" {
		clusterAutoscalerServiceAccount = eksConfig.ClusterAutoscalerServiceAccount
	}
	clusterAutoscalerNamespace := "cluster-autoscaler"
	if eksConfig.ClusterAutoscalerNamespace != "" {
		clusterAutoscalerNamespace = eksConfig.ClusterAutoscalerNamespace
	}

	eksServiceRole, err := iam.NewRole(ctx, "eks-iam-role", &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(`{
			"Version": "2008-10-17",
			"Statement": [{
				"Sid": "",
				"Effect": "Allow",
				"Principal": {
					"Service": "eks.amazonaws.com"
				},
				"Action": "sts:AssumeRole"
			}]
		}`),
	})
	if err != nil {
		return err
	}

	eksPolicyArns := []string{
		"arn:aws:iam::aws:policy/AmazonEKSServicePolicy",
		"arn:aws:iam::aws:policy/AmazonEKSClusterPolicy",
	}
	for _, policyArn := range eksPolicyArns {
		policyName := strings.TrimPrefix(policyArn, "arn:aws:iam::aws:policy/")
		_, err := iam.NewRolePolicyAttachment(ctx, fmt.Sprintf("eks-%s-policy-attachment", policyName), &iam.RolePolicyAttachmentArgs{
			Role:      eksServiceRole.Name,
			PolicyArn: pulumi.String(policyArn),
		})
		if err != nil {
			return err
		}
	}

	nodeGroupRole, err := iam.NewRole(ctx, "nodegroup-iam-role", &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(`{
			"Version": "2012-10-17",
			"Statement": [{
				"Sid": "",
				"Effect": "Allow",
				"Principal": {
					"Service": "ec2.amazonaws.com"
				},
				"Action": "sts:AssumeRole"
			}]
		}`),
	})
	if err != nil {
		return err
	}

	nodeGroupPolicyArns := []string{
		"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
		"arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
		"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
	}
	for _, policyArn := range nodeGroupPolicyArns {
		policyName := strings.TrimPrefix(policyArn, "arn:aws:iam::aws:policy/")
		_, err := iam.NewRolePolicyAttachment(ctx, fmt.Sprintf("nodegroup-%s-policy-attachment", policyName), &iam.RolePolicyAttachmentArgs{
			Role:      nodeGroupRole.Name,
			PolicyArn: pulumi.String(policyArn),
		})
		if err != nil {
			return err
		}
	}

	// create eks cluster
	cluster, err := eks.NewCluster(ctx, "eks-cluster", &eks.ClusterArgs{
		Name:    pulumi.String(clusterName),
		Version: pulumi.String(eksConfig.K8sVersion),
		RoleArn: pulumi.StringInput(eksServiceRole.Arn),
		EnabledClusterLogTypes: pulumi.ToStringArray([]string{
			"api",
			"audit",
			"authenticator",
			"controllerManager",
			"scheduler",
		}),
		VpcConfig: &eks.ClusterVpcConfigArgs{
			SubnetIds:            pulumi.ToStringArrayOutput(eksConfig.SubnetIDs),
			EndpointPublicAccess: pulumi.Bool(true),
			PublicAccessCidrs: pulumi.StringArray{
				pulumi.String("0.0.0.0/0"),
			},
		},
	})
	if err != nil {
		return err
	}

	var nodeGroups []pulumi.Resource
	for _, nodeGroupConfig := range eksConfig.NodeGroupConfig {
		nodeGroup, err := eks.NewNodeGroup(ctx, fmt.Sprintf("node-group-%s", nodeGroupConfig.Name), &eks.NodeGroupArgs{
			ClusterName:         cluster.Name,
			NodeGroupNamePrefix: pulumi.String(nodeGroupConfig.Name),
			NodeRoleArn:         pulumi.StringInput(nodeGroupRole.Arn),
			InstanceTypes:       pulumi.ToStringArray(nodeGroupConfig.InstanceTypes),
			SubnetIds:           pulumi.ToStringArrayOutput(eksConfig.SubnetIDs),
			ScalingConfig: &eks.NodeGroupScalingConfigArgs{
				DesiredSize: pulumi.Int(nodeGroupConfig.DesiredSize),
				MaxSize:     pulumi.Int(nodeGroupConfig.MaxSize),
				MinSize:     pulumi.Int(nodeGroupConfig.MinSize),
			},
		}, pulumi.IgnoreChanges([]string{"scalingConfig.desiredSize"}))
		if err != nil {
			return err
		}

		nodeGroups = append(nodeGroups, nodeGroup)
	}

	// create oidc provider for IRSA https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html
	oidcProvider, err := iam.NewOpenIdConnectProvider(ctx, "eks-oidc-provider", &iam.OpenIdConnectProviderArgs{
		ClientIdLists:   pulumi.StringArray{pulumi.String("sts.amazonaws.com")},
		ThumbprintLists: pulumi.StringArray{pulumi.String(awsRootCAThumbprint)},
		Url:             cluster.Identities.Index(pulumi.Int(0)).Oidcs().Index(pulumi.Int(0)).Issuer().Elem(), // what the fuck
	})
	if err != nil {
		return err
	}

	// create cluster autoscaler iam policy
	clusterAutoscalerPolicyJson, err := json.Marshal(map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			// allow read only actions
			{
				"Action": []string{
					"autoscaling:DescribeAutoScalingGroups",
					"autoscaling:DescribeAutoScalingInstances",
					"autoscaling:DescribeLaunchConfigurations",
					"autoscaling:DescribeTags",
					"ec2:DescribeLaunchTemplateVersions",
					"ec2:DescribeInstanceTypes",
				},
				"Effect":   "Allow",
				"Resource": "*",
			},
			// allow autoscaling for only this specific eks cluster
			{
				"Action": []string{
					"autoscaling:SetDesiredCapacity",
					"autoscaling:TerminateInstanceInAutoScalingGroup",
					"autoscaling:UpdateAutoScalingGroup",
				},
				"Effect":   "Allow",
				"Resource": "*",
				"Condition": map[string]interface{}{
					"StringEquals": map[string]string{
						fmt.Sprintf("autoscaling:ResourceTag/kubernetes.io/cluster/%s", clusterName): "owned",
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}

	clusterAutoscalerPolicy, err := iam.NewPolicy(ctx, "cluster-autoscaler-policy", &iam.PolicyArgs{
		Name:        pulumi.String(fmt.Sprintf("cluster-autoscaler-policy-%s", clusterName)),
		Description: pulumi.String(fmt.Sprintf("cluster autoscaler policy for %s eks cluster", clusterName)),
		Policy:      pulumi.String(clusterAutoscalerPolicyJson),
	})
	if err != nil {
		return err
	}

	// create cluster autoscaler iam role with IRSA
	clusterAutoscalerRole, err := iam.NewRole(ctx, "cluster-autoscaler-role", &iam.RoleArgs{
		Name: pulumi.String(fmt.Sprintf("cluster-autoscaler-role-%s", clusterName)),
		AssumeRolePolicy: createIrsaAssumeRolePolicy(oidcProvider, clusterAutoscalerNamespace, clusterAutoscalerServiceAccount),
	})
	if err != nil {
		return err
	}
	_, err = iam.NewRolePolicyAttachment(ctx, "cluster-autoscaler-role-policy-attachment", &iam.RolePolicyAttachmentArgs{
		Role:      clusterAutoscalerRole.Name,
		PolicyArn: clusterAutoscalerPolicy.Arn,
	})
	if err != nil {
		return err
	}

	return nil
}

// creates an iam assume role policy for IRSA
// https://docs.aws.amazon.com/eks/latest/userguide/create-service-account-iam-policy-and-role.html
func createIrsaAssumeRolePolicy(oidcProvider *iam.OpenIdConnectProvider, namespace string, serviceAccount string) pulumi.Output {
	return pulumi.All(oidcProvider.Arn, oidcProvider.Url).ApplyT(func(args []interface{}) (string, error) {
		arn := args[0].(string)
		provider := strings.TrimLeft(args[1].(string), "https://")
		policyByteArray, err := json.Marshal(map[string]interface{}{
			"Version": "2012-10-17",
			"Statement": []map[string]interface{}{
				{
					"Action": "sts:AssumeRoleWithWebIdentity",
					"Effect": "Allow",
					"Principal": map[string]interface{}{
						"Federated": arn,
					},
					"Condition": map[string]interface{}{
						"StringEquals": map[string]string{
							fmt.Sprintf("%s:sub", provider): fmt.Sprintf("system:serviceaccount:%s:%s", namespace, serviceAccount),
						},
					},
				},
			},
		})
		return string(policyByteArray), err
	})
}
