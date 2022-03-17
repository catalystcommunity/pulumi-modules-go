package vpc

import (
	"fmt"
	"github.com/pulumi/pulumi-aws/sdk/v4/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type VpcConfigInput struct {
	AvailabilityZones []AvailabilityZoneInput `json:"availability-zones"`
	Cidr              string                  `json:"cidr"`
}

type AvailabilityZoneInput struct {
	AzName            string `json:"az-name"`
	PrivateSubnetCidr string `json:"private-subnet-cidr"`
	PublicSubnetCidr  string `json:"public-subnet-cidr"`
}

type VpcInfrastructureOutput struct {
	VpcID            pulumi.StringOutput
	PrivateSubnetIDs []pulumi.StringOutput
	PublicSubnetIDs  []pulumi.StringOutput
	NatGatewayIPs    []pulumi.StringOutput
}

func CreateVpcInfrastructure(ctx *pulumi.Context, vpcConfig VpcConfigInput) (*VpcInfrastructureOutput, error) {
	var output VpcInfrastructureOutput

	name := ctx.Stack()

	// create the vpc
	vpc, err := ec2.NewVpc(ctx, fmt.Sprintf("%s-vpc", name), &ec2.VpcArgs{
		CidrBlock: pulumi.String(vpcConfig.Cidr),
		Tags: pulumi.StringMap{
			"Name": pulumi.String(name),
		},
	})
	if err != nil {
		return nil, err
	}

	output.VpcID = vpc.ID().ToStringOutput()

	// create internet gateway
	internetGateway, err := ec2.NewInternetGateway(ctx, fmt.Sprintf("%s-internet-gateway", name), &ec2.InternetGatewayArgs{
		VpcId: vpc.ID(),
	})
	if err != nil {
		return nil, err
	}

	for i, az := range vpcConfig.AvailabilityZones {
		// create public subnets
		publicSubnet, err := ec2.NewSubnet(ctx, fmt.Sprintf("%s-public-subnet-%d", name, i), &ec2.SubnetArgs{
			VpcId:            vpc.ID(),
			CidrBlock:        pulumi.String(az.PublicSubnetCidr),
			AvailabilityZone: pulumi.String(az.AzName),
			Tags: pulumi.StringMap{
				fmt.Sprintf("kubernetes.io/cluster/%s", name): pulumi.String("owned"),
				"kubernetes.io/role/elb":                      pulumi.String("1"),
			},
		})
		if err != nil {
			return nil, err
		}

		output.PublicSubnetIDs = append(output.PublicSubnetIDs, publicSubnet.ID().ToStringOutput())

		// create public subnet route tables
		publicRouteTable, err := ec2.NewRouteTable(ctx, fmt.Sprintf("%s-public-route-table-%d", name, i), &ec2.RouteTableArgs{
			VpcId: vpc.ID(),
		})
		if err != nil {
			return nil, err
		}

		// default public route
		_, err = ec2.NewRoute(ctx, fmt.Sprintf("%s-public-route-%d", name, i), &ec2.RouteArgs{
			RouteTableId:         publicRouteTable.ID(),
			DestinationCidrBlock: pulumi.String("0.0.0.0/0"),
			GatewayId:            internetGateway.ID(),
		})
		if err != nil {
			return nil, err
		}

		// associate route table to new subnet
		_, err = ec2.NewRouteTableAssociation(ctx, fmt.Sprintf("%s-public-route-table-association-%d", name, i), &ec2.RouteTableAssociationArgs{
			SubnetId:     publicSubnet.ID(),
			RouteTableId: publicRouteTable.ID(),
		})
		if err != nil {
			return nil, err
		}

		// create nat gateway public ip
		natGatewayIp, err := ec2.NewEip(ctx, fmt.Sprintf("%s-elastic-ip-%d", name, i), &ec2.EipArgs{
			Vpc: pulumi.Bool(true),
		})
		if err != nil {
			return nil, err
		}

		output.NatGatewayIPs = append(output.NatGatewayIPs, natGatewayIp.ID().ToStringOutput())

		// create nat gateway
		natGateway, err := ec2.NewNatGateway(ctx, fmt.Sprintf("%s-nat-gateway-%d", name, i), &ec2.NatGatewayArgs{
			AllocationId: natGatewayIp.ID(),
			SubnetId:     publicSubnet.ID(),
		})
		if err != nil {
			return nil, err
		}

		// create private subnets
		privateSubnet, err := ec2.NewSubnet(ctx, fmt.Sprintf("%s-private-subnet-%d", name, i), &ec2.SubnetArgs{
			VpcId:            vpc.ID(),
			CidrBlock:        pulumi.String(az.PrivateSubnetCidr),
			AvailabilityZone: pulumi.String(az.AzName),
			Tags: pulumi.StringMap{
				"kubernetes.io/cluster/cluster-name": pulumi.String(name),
			},
		})
		if err != nil {
			return nil, err
		}

		output.PrivateSubnetIDs = append(output.PrivateSubnetIDs, privateSubnet.ID().ToStringOutput())

		// create private subnet route tables
		privateRouteTable, err := ec2.NewRouteTable(ctx, fmt.Sprintf("%s-private-route-table-%d", name, i), &ec2.RouteTableArgs{
			VpcId: vpc.ID(),
		})
		if err != nil {
			return nil, err
		}

		// default private route
		_, err = ec2.NewRoute(ctx, fmt.Sprintf("%s-private-route-%d", name, i), &ec2.RouteArgs{
			RouteTableId:         privateRouteTable.ID(),
			DestinationCidrBlock: pulumi.String("0.0.0.0/0"),
			NatGatewayId:         natGateway.ID(),
		})
		if err != nil {
			return nil, err
		}

		// associate route table to new subnet
		_, err = ec2.NewRouteTableAssociation(ctx, fmt.Sprintf("%s-private-route-table-association-%d", name, i), &ec2.RouteTableAssociationArgs{
			SubnetId:     privateSubnet.ID(),
			RouteTableId: privateRouteTable.ID(),
		})
		if err != nil {
			return nil, err
		}
	}

	return &output, nil
}
