// Copyright (c) 2017, 2019, Oracle and/or its affiliates. All rights reserved.

package provider

import (
	"context"

	"github.com/hashicorp/terraform/helper/schema"
	oci_dns "github.com/oracle/oci-go-sdk/dns"
)

func DnsSteeringPolicyAttachmentDataSource() *schema.Resource {
	return &schema.Resource{
		Read: readSingularDnsSteeringPolicyAttachment,
		Schema: map[string]*schema.Schema{
			"steering_policy_attachment_id": {
				Type:     schema.TypeString,
				Required: true,
			},
			// Computed
			"compartment_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"display_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"domain_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"rtypes": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"self": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"state": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"steering_policy_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"time_created": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"zone_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func readSingularDnsSteeringPolicyAttachment(d *schema.ResourceData, m interface{}) error {
	sync := &DnsSteeringPolicyAttachmentDataSourceCrud{}
	sync.D = d
	sync.Client = m.(*OracleClients).dnsClient

	return ReadResource(sync)
}

type DnsSteeringPolicyAttachmentDataSourceCrud struct {
	D      *schema.ResourceData
	Client *oci_dns.DnsClient
	Res    *oci_dns.GetSteeringPolicyAttachmentResponse
}

func (s *DnsSteeringPolicyAttachmentDataSourceCrud) VoidState() {
	s.D.SetId("")
}

func (s *DnsSteeringPolicyAttachmentDataSourceCrud) Get() error {
	request := oci_dns.GetSteeringPolicyAttachmentRequest{}

	if steeringPolicyAttachmentId, ok := s.D.GetOkExists("steering_policy_attachment_id"); ok {
		tmp := steeringPolicyAttachmentId.(string)
		request.SteeringPolicyAttachmentId = &tmp
	}

	request.RequestMetadata.RetryPolicy = getRetryPolicy(false, "dns")

	response, err := s.Client.GetSteeringPolicyAttachment(context.Background(), request)
	if err != nil {
		return err
	}

	s.Res = &response
	return nil
}

func (s *DnsSteeringPolicyAttachmentDataSourceCrud) SetData() error {
	if s.Res == nil {
		return nil
	}

	s.D.SetId(*s.Res.Id)

	if s.Res.CompartmentId != nil {
		s.D.Set("compartment_id", *s.Res.CompartmentId)
	}

	if s.Res.DisplayName != nil {
		s.D.Set("display_name", *s.Res.DisplayName)
	}

	if s.Res.DomainName != nil {
		s.D.Set("domain_name", *s.Res.DomainName)
	}

	s.D.Set("rtypes", s.Res.Rtypes)

	if s.Res.Self != nil {
		s.D.Set("self", *s.Res.Self)
	}

	s.D.Set("state", s.Res.LifecycleState)

	if s.Res.SteeringPolicyId != nil {
		s.D.Set("steering_policy_id", *s.Res.SteeringPolicyId)
	}

	if s.Res.TimeCreated != nil {
		s.D.Set("time_created", s.Res.TimeCreated.String())
	}

	if s.Res.ZoneId != nil {
		s.D.Set("zone_id", *s.Res.ZoneId)
	}

	return nil
}
