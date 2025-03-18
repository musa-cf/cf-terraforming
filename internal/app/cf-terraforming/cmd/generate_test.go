package cmd

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc/v2"
	cfv0 "github.com/cloudflare/cloudflare-go"
	"github.com/cloudflare/cloudflare-go/v4"
	"github.com/cloudflare/cloudflare-go/v4/option"
	"github.com/dnaeon/go-vcr/cassette"
	"github.com/dnaeon/go-vcr/recorder"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/spf13/viper"

	"github.com/stretchr/testify/assert"
)

var (
	// listOfString is an example representation of a key where the value is a
	// list of string values.
	//
	//   resource "example" "example" {
	//     attr = [ "b", "c", "d"]
	//   }
	listOfString = []interface{}{"b", "c", "d"}

	// configBlockOfStrings is an example of where a key is a "block" assignment
	// in HCL.
	//
	//   resource "example" "example" {
	//     attr = {
	//       c = "d"
	//       e = "f"
	//     }
	//   }
	configBlockOfStrings = map[string]interface{}{
		"c": "d",
		"e": "f",
	}

	cloudflareTestZoneID    = "0da42c8d2132a9ddaf714f9e7c920711"
	cloudflareTestAccountID = "f037e56e89293a057740de681ac9abbe"
)

func TestGenerate_writeAttrLine(t *testing.T) {
	multilineListOfStrings := heredoc.Doc(`
		a = ["b", "c", "d"]
	`)
	multilineBlock := heredoc.Doc(`
		a = {
		  c = "d"
		  e = "f"
		}
	`)
	tests := map[string]struct {
		key   string
		value interface{}
		want  string
	}{
		"value is string":           {key: "a", value: "b", want: fmt.Sprintf("a = %q\n", "b")},
		"value is int":              {key: "a", value: 1, want: "a = 1\n"},
		"value is float":            {key: "a", value: 1.0, want: "a = 1\n"},
		"value is bool":             {key: "a", value: true, want: "a = true\n"},
		"value is list of strings":  {key: "a", value: listOfString, want: multilineListOfStrings},
		"value is block of strings": {key: "a", value: configBlockOfStrings, want: multilineBlock},
		"value is nil":              {key: "a", value: nil, want: ""},
	}

	for name, tc := range tests {
		f := hclwrite.NewEmptyFile()
		t.Run(name, func(t *testing.T) {
			writeAttrLine(tc.key, tc.value, "", f.Body())
			assert.Equal(t, tc.want, string(f.Bytes()))
		})
	}
}

func TestGenerate_ResourceNotSupported(t *testing.T) {
	output, err := executeCommandC(rootCmd, "generate", "--resource-type", "notreal")
	assert.Nil(t, err)
	assert.Equal(t, output, `"notreal" is not yet supported for automatic generation`)
}

func TestResourceGeneration(t *testing.T) {
	tests := map[string]struct {
		identiferType    string
		resourceType     string
		testdataFilename string
	}{
		"cloudflare access application simple (account)":     {identiferType: "account", resourceType: "cloudflare_access_application", testdataFilename: "cloudflare_access_application_simple_account"},
		"cloudflare access application with CORS (account)":  {identiferType: "account", resourceType: "cloudflare_access_application", testdataFilename: "cloudflare_access_application_with_cors_account"},
		"cloudflare access IdP OAuth (account)":              {identiferType: "account", resourceType: "cloudflare_access_identity_provider", testdataFilename: "cloudflare_access_identity_provider_oauth_account"},
		"cloudflare access IdP OAuth (zone)":                 {identiferType: "zone", resourceType: "cloudflare_access_identity_provider", testdataFilename: "cloudflare_access_identity_provider_oauth_zone"},
		"cloudflare access IdP OTP (account)":                {identiferType: "account", resourceType: "cloudflare_access_identity_provider", testdataFilename: "cloudflare_access_identity_provider_otp_account"},
		"cloudflare access IdP OTP (zone)":                   {identiferType: "zone", resourceType: "cloudflare_access_identity_provider", testdataFilename: "cloudflare_access_identity_provider_otp_zone"},
		"cloudflare access rule (account)":                   {identiferType: "account", resourceType: "cloudflare_access_rule", testdataFilename: "cloudflare_access_rule_account"},
		"cloudflare access rule (zone)":                      {identiferType: "zone", resourceType: "cloudflare_access_rule", testdataFilename: "cloudflare_access_rule_zone"},
		"cloudflare account member":                          {identiferType: "account", resourceType: "cloudflare_account_member", testdataFilename: "cloudflare_account_member"},
		"cloudflare api shield":                              {identiferType: "zone", resourceType: "cloudflare_api_shield", testdataFilename: "cloudflare_api_shield"},
		"cloudflare argo":                                    {identiferType: "zone", resourceType: "cloudflare_argo", testdataFilename: "cloudflare_argo"},
		"cloudflare bot management":                          {identiferType: "zone", resourceType: "cloudflare_bot_management", testdataFilename: "cloudflare_bot_management"},
		"cloudflare BYO IP prefix":                           {identiferType: "account", resourceType: "cloudflare_byo_ip_prefix", testdataFilename: "cloudflare_byo_ip_prefix"},
		"cloudflare certificate pack":                        {identiferType: "zone", resourceType: "cloudflare_certificate_pack", testdataFilename: "cloudflare_certificate_pack_acm"},
		"cloudflare custom hostname fallback origin":         {identiferType: "zone", resourceType: "cloudflare_custom_hostname_fallback_origin", testdataFilename: "cloudflare_custom_hostname_fallback_origin"},
		"cloudflare custom hostname":                         {identiferType: "zone", resourceType: "cloudflare_custom_hostname", testdataFilename: "cloudflare_custom_hostname"},
		"cloudflare custom pages (account)":                  {identiferType: "account", resourceType: "cloudflare_custom_pages", testdataFilename: "cloudflare_custom_pages_account"},
		"cloudflare custom pages (zone)":                     {identiferType: "zone", resourceType: "cloudflare_custom_pages", testdataFilename: "cloudflare_custom_pages_zone"},
		"cloudflare filter":                                  {identiferType: "zone", resourceType: "cloudflare_filter", testdataFilename: "cloudflare_filter"},
		"cloudflare firewall rule":                           {identiferType: "zone", resourceType: "cloudflare_firewall_rule", testdataFilename: "cloudflare_firewall_rule"},
		"cloudflare health check":                            {identiferType: "zone", resourceType: "cloudflare_healthcheck", testdataFilename: "cloudflare_healthcheck"},
		"cloudflare list (asn)":                              {identiferType: "account", resourceType: "cloudflare_list", testdataFilename: "cloudflare_list_asn"},
		"cloudflare list (hostname)":                         {identiferType: "account", resourceType: "cloudflare_list", testdataFilename: "cloudflare_list_hostname"},
		"cloudflare list (ip)":                               {identiferType: "account", resourceType: "cloudflare_list", testdataFilename: "cloudflare_list_ip"},
		"cloudflare list (redirect)":                         {identiferType: "account", resourceType: "cloudflare_list", testdataFilename: "cloudflare_list_redirect"},
		"cloudflare load balancer monitor":                   {identiferType: "account", resourceType: "cloudflare_load_balancer_monitor", testdataFilename: "cloudflare_load_balancer_monitor"},
		"cloudflare load balancer pool":                      {identiferType: "account", resourceType: "cloudflare_load_balancer_pool", testdataFilename: "cloudflare_load_balancer_pool"},
		"cloudflare load balancer":                           {identiferType: "zone", resourceType: "cloudflare_load_balancer", testdataFilename: "cloudflare_load_balancer"},
		"cloudflare logpush jobs with filter":                {identiferType: "zone", resourceType: "cloudflare_logpush_job", testdataFilename: "cloudflare_logpush_job_with_filter"},
		"cloudflare logpush jobs":                            {identiferType: "zone", resourceType: "cloudflare_logpush_job", testdataFilename: "cloudflare_logpush_job"},
		"cloudflare managed headers":                         {identiferType: "zone", resourceType: "cloudflare_managed_headers", testdataFilename: "cloudflare_managed_headers"},
		"cloudflare origin CA certificate":                   {identiferType: "zone", resourceType: "cloudflare_origin_ca_certificate", testdataFilename: "cloudflare_origin_ca_certificate"},
		"cloudflare page rule":                               {identiferType: "zone", resourceType: "cloudflare_page_rule", testdataFilename: "cloudflare_page_rule"},
		"cloudflare rate limit":                              {identiferType: "zone", resourceType: "cloudflare_rate_limit", testdataFilename: "cloudflare_rate_limit"},
		"cloudflare record CAA":                              {identiferType: "zone", resourceType: "cloudflare_record", testdataFilename: "cloudflare_record_caa"},
		"cloudflare record PTR":                              {identiferType: "zone", resourceType: "cloudflare_record", testdataFilename: "cloudflare_record_ptr"},
		"cloudflare record simple":                           {identiferType: "zone", resourceType: "cloudflare_record", testdataFilename: "cloudflare_record"},
		"cloudflare record subdomain":                        {identiferType: "zone", resourceType: "cloudflare_record", testdataFilename: "cloudflare_record_subdomain"},
		"cloudflare record TXT SPF":                          {identiferType: "zone", resourceType: "cloudflare_record", testdataFilename: "cloudflare_record_txt_spf"},
		"cloudflare ruleset (ddos_l7)":                       {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone_ddos_l7"},
		"cloudflare ruleset (http_log_custom_fields)":        {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone_http_log_custom_fields"},
		"cloudflare ruleset (http_ratelimit)":                {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone_http_ratelimit"},
		"cloudflare ruleset (http_request_cache_settings)":   {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_http_request_cache_settings"},
		"cloudflare ruleset (http_request_firewall_custom)":  {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone_http_request_firewall_custom"},
		"cloudflare ruleset (http_request_firewall_managed)": {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone_http_request_firewall_managed"},
		"cloudflare ruleset (http_request_late_transform)":   {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone_http_request_late_transform"},
		"cloudflare ruleset (http_request_sanitize)":         {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone_http_request_sanitize"},
		"cloudflare ruleset (no configuration)":              {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone_no_configuration"},
		"cloudflare ruleset (override remapping = disabled)": {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_override_remapping_disabled"},
		"cloudflare ruleset (override remapping = enabled)":  {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_override_remapping_enabled"},
		"cloudflare ruleset (rewrite to empty query string)": {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone_rewrite_to_empty_query_parameter"},
		"cloudflare ruleset":                                 {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone"},
		"cloudflare spectrum application":                    {identiferType: "zone", resourceType: "cloudflare_spectrum_application", testdataFilename: "cloudflare_spectrum_application"},
		"cloudflare teams list":                              {identiferType: "account", resourceType: "cloudflare_teams_list", testdataFilename: "cloudflare_teams_list"},
		"cloudflare teams location":                          {identiferType: "account", resourceType: "cloudflare_teams_location", testdataFilename: "cloudflare_teams_location"},
		"cloudflare teams proxy endpoint":                    {identiferType: "account", resourceType: "cloudflare_teams_proxy_endpoint", testdataFilename: "cloudflare_teams_proxy_endpoint"},
		"cloudflare teams rule":                              {identiferType: "account", resourceType: "cloudflare_teams_rule", testdataFilename: "cloudflare_teams_rule"},
		"cloudflare tunnel":                                  {identiferType: "account", resourceType: "cloudflare_tunnel", testdataFilename: "cloudflare_tunnel"},
		"cloudflare turnstile_widget":                        {identiferType: "account", resourceType: "cloudflare_turnstile_widget", testdataFilename: "cloudflare_turnstile_widget"},
		"cloudflare turnstile_widget_no_domains":             {identiferType: "account", resourceType: "cloudflare_turnstile_widget", testdataFilename: "cloudflare_turnstile_widget_no_domains"},
		"cloudflare url normalization settings":              {identiferType: "zone", resourceType: "cloudflare_url_normalization_settings", testdataFilename: "cloudflare_url_normalization_settings"},
		"cloudflare user agent blocking rule":                {identiferType: "zone", resourceType: "cloudflare_user_agent_blocking_rule", testdataFilename: "cloudflare_user_agent_blocking_rule"},
		"cloudflare waiting room":                            {identiferType: "zone", resourceType: "cloudflare_waiting_room", testdataFilename: "cloudflare_waiting_room"},
		"cloudflare waiting room event":                      {identiferType: "zone", resourceType: "cloudflare_waiting_room_event", testdataFilename: "cloudflare_waiting_room_event"},
		"cloudflare waiting room rules":                      {identiferType: "zone", resourceType: "cloudflare_waiting_room_rules", testdataFilename: "cloudflare_waiting_room_rules"},
		"cloudflare waiting room settings":                   {identiferType: "zone", resourceType: "cloudflare_waiting_room_settings", testdataFilename: "cloudflare_waiting_room_settings"},
		"cloudflare worker route":                            {identiferType: "zone", resourceType: "cloudflare_worker_route", testdataFilename: "cloudflare_worker_route"},
		"cloudflare workers kv namespace":                    {identiferType: "account", resourceType: "cloudflare_workers_kv_namespace", testdataFilename: "cloudflare_workers_kv_namespace"},
		"cloudflare zone lockdown":                           {identiferType: "zone", resourceType: "cloudflare_zone_lockdown", testdataFilename: "cloudflare_zone_lockdown"},
		"cloudflare zone settings override":                  {identiferType: "zone", resourceType: "cloudflare_zone_settings_override", testdataFilename: "cloudflare_zone_settings_override"},
		"cloudflare tiered cache":                            {identiferType: "zone", resourceType: "cloudflare_tiered_cache", testdataFilename: "cloudflare_tiered_cache"},

		// "cloudflare access group (account)": {identiferType: "account", resourceType: "cloudflare_access_group", testdataFilename: "cloudflare_access_group_account"},
		// "cloudflare access group (zone)":    {identiferType: "zone", resourceType: "cloudflare_access_group", testdataFilename: "cloudflare_access_group_zone"},
		// "cloudflare custom certificates":    {identiferType: "zone", resourceType: "cloudflare_custom_certificates", testdataFilename: "cloudflare_custom_certificates"},
		// "cloudflare custom SSL":             {identiferType: "zone", resourceType: "cloudflare_custom_ssl", testdataFilename: "cloudflare_custom_ssl"},
		// "cloudflare load balancer pool":     {identiferType: "account", resourceType: "cloudflare_load_balancer_pool", testdataFilename: "cloudflare_load_balancer_pool"},
		// "cloudflare worker cron trigger":    {identiferType: "zone", resourceType: "cloudflare_worker_cron_trigger", testdataFilename: "cloudflare_worker_cron_trigger"},
		// "cloudflare zone":                   {identiferType: "zone", resourceType: "cloudflare_zone", testdataFilename: "cloudflare_zone"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Reset the environment variables used in test to ensure we don't
			// have both present at once.
			viper.Set("zone", "")
			viper.Set("account", "")

			var r *recorder.Recorder
			var err error
			if os.Getenv("OVERWRITE_VCR_CASSETTES") == "true" {
				r, err = recorder.NewAsMode("../../../../testdata/cloudflare/v4/"+tc.testdataFilename, recorder.ModeRecording, http.DefaultTransport)
			} else {
				r, err = recorder.New("../../../../testdata/cloudflare/v4/" + tc.testdataFilename)
			}

			if err != nil {
				log.Fatal(err)
			}
			defer func() {
				err := r.Stop()
				if err != nil {
					log.Fatal(err)
				}
			}()

			r.AddFilter(func(i *cassette.Interaction) error {
				// Sensitive HTTP headers
				delete(i.Request.Headers, "X-Auth-Email")
				delete(i.Request.Headers, "X-Auth-Key")
				delete(i.Request.Headers, "Authorization")

				// HTTP request headers that we don't need to assert against
				delete(i.Request.Headers, "User-Agent")

				// HTTP response headers that we don't need to assert against
				delete(i.Response.Headers, "Cf-Cache-Status")
				delete(i.Response.Headers, "Cf-Ray")
				delete(i.Response.Headers, "Date")
				delete(i.Response.Headers, "Server")
				delete(i.Response.Headers, "Set-Cookie")
				delete(i.Response.Headers, "X-Envoy-Upstream-Service-Time")

				if os.Getenv("CLOUDFLARE_DOMAIN") != "" {
					i.Response.Body = strings.ReplaceAll(i.Response.Body, os.Getenv("CLOUDFLARE_DOMAIN"), "example.com")
				}

				return nil
			})

			output := ""

			if tc.identiferType == "account" {
				viper.Set("account", cloudflareTestAccountID)
				apiV0, _ = cfv0.New(viper.GetString("key"), viper.GetString("email"), cfv0.HTTPClient(
					&http.Client{
						Transport: r,
					},
				))

				output, _ = executeCommandC(rootCmd, "generate", "--resource-type", tc.resourceType, "--account", cloudflareTestAccountID)
			} else {
				viper.Set("zone", cloudflareTestZoneID)
				apiV0, _ = cfv0.New(viper.GetString("key"), viper.GetString("email"), cfv0.HTTPClient(
					&http.Client{
						Transport: r,
					},
				))

				output, _ = executeCommandC(rootCmd, "generate", "--resource-type", tc.resourceType, "--zone", cloudflareTestZoneID)
			}

			expected := testDataFile("v4", tc.testdataFilename)
			assert.Equal(t, strings.TrimRight(expected, "\n"), strings.TrimRight(output, "\n"))
		})
	}
}

func TestResourceGenerationV5(t *testing.T) {
	t.Skip("skip until the v5 provider is fully supported")

	tests := map[string]struct {
		identiferType    string
		resourceType     string
		testdataFilename string
	}{
		// "cloudflare access application simple (account)":     {identiferType: "account", resourceType: "cloudflare_access_application", testdataFilename: "cloudflare_access_application_simple_account"},
		// "cloudflare access application with CORS (account)":  {identiferType: "account", resourceType: "cloudflare_access_application", testdataFilename: "cloudflare_access_application_with_cors_account"},
		// "cloudflare access IdP OAuth (account)":              {identiferType: "account", resourceType: "cloudflare_access_identity_provider", testdataFilename: "cloudflare_access_identity_provider_oauth_account"},
		// "cloudflare access IdP OAuth (zone)":                 {identiferType: "zone", resourceType: "cloudflare_access_identity_provider", testdataFilename: "cloudflare_access_identity_provider_oauth_zone"},
		// "cloudflare access IdP OTP (account)":                {identiferType: "account", resourceType: "cloudflare_access_identity_provider", testdataFilename: "cloudflare_access_identity_provider_otp_account"},
		// "cloudflare access IdP OTP (zone)":                   {identiferType: "zone", resourceType: "cloudflare_access_identity_provider", testdataFilename: "cloudflare_access_identity_provider_otp_zone"},
		// "cloudflare access rule (account)":                   {identiferType: "account", resourceType: "cloudflare_access_rule", testdataFilename: "cloudflare_access_rule_account"},
		"cloudflare account": {identiferType: "account", resourceType: "cloudflare_account", testdataFilename: "cloudflare_account"},
		// "cloudflare access rule (zone)":                      {identiferType: "zone", resourceType: "cloudflare_access_rule", testdataFilename: "cloudflare_access_rule_zone"},
		"cloudflare account subscription": {identiferType: "account", resourceType: "cloudflare_account_subscription", testdataFilename: "cloudflare_account_subscription"},
		"cloudflare address map":          {identiferType: "account", resourceType: "cloudflare_address_map", testdataFilename: "cloudflare_address_map"},
		"cloudflare account member":       {identiferType: "account", resourceType: "cloudflare_account_member", testdataFilename: "cloudflare_account_member"},
		"cloudflare api shield operation": {identiferType: "zone", resourceType: "cloudflare_api_shield_operation", testdataFilename: "cloudflare_api_shield_operation"},
		// "cloudflare argo":                                    {identiferType: "zone", resourceType: "cloudflare_argo", testdataFilename: "cloudflare_argo"},
		// "cloudflare bot management":                          {identiferType: "zone", resourceType: "cloudflare_bot_management", testdataFilename: "cloudflare_bot_management"},
		// "cloudflare BYO IP prefix":                           {identiferType: "account", resourceType: "cloudflare_byo_ip_prefix", testdataFilename: "cloudflare_byo_ip_prefix"},
		"cloudflare certificate pack":                {identiferType: "zone", resourceType: "cloudflare_certificate_pack", testdataFilename: "cloudflare_certificate_pack"},
		"cloudflare_content_scanning_expression":     {identiferType: "zone", resourceType: "cloudflare_content_scanning_expression", testdataFilename: "cloudflare_content_scanning_expression"},
		"cloudflare custom hostname fallback origin": {identiferType: "zone", resourceType: "cloudflare_custom_hostname_fallback_origin", testdataFilename: "cloudflare_custom_hostname_fallback_origin"},
		"cloudflare custom hostname":                 {identiferType: "zone", resourceType: "cloudflare_custom_hostname", testdataFilename: "cloudflare_custom_hostname"},
		// "cloudflare custom pages (account)":                  {identiferType: "account", resourceType: "cloudflare_custom_pages", testdataFilename: "cloudflare_custom_pages_account"},
		// "cloudflare custom pages (zone)":                     {identiferType: "zone", resourceType: "cloudflare_custom_pages", testdataFilename: "cloudflare_custom_pages_zone"},
		"cloudflare email routing address":   {identiferType: "account", resourceType: "cloudflare_email_routing_address", testdataFilename: "cloudflare_email_routing_address"},
		"cloudflare email routing catch all": {identiferType: "zone", resourceType: "cloudflare_email_routing_catch_all", testdataFilename: "cloudflare_email_routing_catch_all"},
		"cloudflare email routing dns":       {identiferType: "zone", resourceType: "cloudflare_email_routing_dns", testdataFilename: "cloudflare_email_routing_dns"},
		//"cloudflare email routing rule":          {identiferType: "zone", resourceType: "cloudflare_email_routing_rule", testdataFilename: "cloudflare_email_routing_rule"},
		"cloudflare email routing settings":      {identiferType: "zone", resourceType: "cloudflare_email_routing_settings", testdataFilename: "cloudflare_email_routing_settings"},
		"cloudflare email security block sender": {identiferType: "account", resourceType: "cloudflare_email_security_block_sender", testdataFilename: "cloudflare_email_security_block_sender"},
		// "cloudflare email security impersonation registry": {identiferType: "account", resourceType: "cloudflare_email_security_impersonation_registry", testdataFilename: "cloudflare_email_security_impersonation_registry"},
		// "cloudflare email security trusted domains":        {identiferType: "account", resourceType: "cloudflare_email_security_trusted_domains", testdataFilename: "cloudflare_email_security_trusted_domains"},
		// "cloudflare filter":                                  {identiferType: "zone", resourceType: "cloudflare_filter", testdataFilename: "cloudflare_filter"},
		// "cloudflare firewall rule":                           {identiferType: "zone", resourceType: "cloudflare_firewall_rule", testdataFilename: "cloudflare_firewall_rule"},
		"cloudflare health check": {identiferType: "zone", resourceType: "cloudflare_healthcheck", testdataFilename: "cloudflare_healthcheck"},
		//"cloudflare hostname tls setting": {identiferType: "zone", resourceType: "cloudflare_hostname_tls_setting", testdataFilename: "cloudflare_hostname_tls_setting"},
		"cloudflare mtls certificate": {identiferType: "account", resourceType: "cloudflare_mtls_certificate", testdataFilename: "cloudflare_mtls_certificate"},
		// "cloudflare list (asn)":                              {identiferType: "account", resourceType: "cloudflare_list", testdataFilename: "cloudflare_list_asn"},
		// "cloudflare list (hostname)":                         {identiferType: "account", resourceType: "cloudflare_list", testdataFilename: "cloudflare_list_hostname"},
		// "cloudflare list (ip)":                               {identiferType: "account", resourceType: "cloudflare_list", testdataFilename: "cloudflare_list_ip"},
		// "cloudflare list (redirect)":                         {identiferType: "account", resourceType: "cloudflare_list", testdataFilename: "cloudflare_list_redirect"},
		// "cloudflare load balancer monitor":                   {identiferType: "account", resourceType: "cloudflare_load_balancer_monitor", testdataFilename: "cloudflare_load_balancer_monitor"},
		// "cloudflare load balancer pool":                      {identiferType: "account", resourceType: "cloudflare_load_balancer_pool", testdataFilename: "cloudflare_load_balancer_pool"},
		// "cloudflare load balancer":                           {identiferType: "zone", resourceType: "cloudflare_load_balancer", testdataFilename: "cloudflare_load_balancer"},
		// "cloudflare logpush jobs with filter":                {identiferType: "zone", resourceType: "cloudflare_logpush_job", testdataFilename: "cloudflare_logpush_job_with_filter"},
		// "cloudflare logpush jobs":                            {identiferType: "zone", resourceType: "cloudflare_logpush_job", testdataFilename: "cloudflare_logpush_job"},
		"cloudflare managed transforms":    {identiferType: "zone", resourceType: "cloudflare_managed_transforms", testdataFilename: "cloudflare_managed_transforms"},
		"cloudflare origin ca certificate": {identiferType: "zone", resourceType: "cloudflare_origin_ca_certificate", testdataFilename: "cloudflare_origin_ca_certificate"},
		// "cloudflare page rule":                               {identiferType: "zone", resourceType: "cloudflare_page_rule", testdataFilename: "cloudflare_page_rule"},
		// "cloudflare rate limit":                              {identiferType: "zone", resourceType: "cloudflare_rate_limit", testdataFilename: "cloudflare_rate_limit"},
		"cloudflare d1 database":  {identiferType: "account", resourceType: "cloudflare_d1_database", testdataFilename: "cloudflare_d1_database"},
		"cloudflare dns firewall": {identiferType: "account", resourceType: "cloudflare_dns_firewall", testdataFilename: "cloudflare_dns_firewall"},
		// "cloudflare dns record CAA":                          {identiferType: "zone", resourceType: "cloudflare_dns_record", testdataFilename: "cloudflare_dns_record_caa"},
		// "cloudflare dns record PTR":                          {identiferType: "zone", resourceType: "cloudflare_dns_record", testdataFilename: "cloudflare_dns_record_ptr"},
		"cloudflare dns record simple": {identiferType: "zone", resourceType: "cloudflare_dns_record", testdataFilename: "cloudflare_dns_record"},
		// "cloudflare dns record subdomain":                    {identiferType: "zone", resourceType: "cloudflare_dns_record", testdataFilename: "cloudflare_dns_record_subdomain"},
		// "cloudflare dns record TXT SPF":                      {identiferType: "zone", resourceType: "cloudflare_dns_record", testdataFilename: "cloudflare_dns_record_txt_spf"},
		"cloudflare dns zone transfers acl":       {identiferType: "account", resourceType: "cloudflare_dns_zone_transfers_acl", testdataFilename: "cloudflare_dns_zone_transfers_acl"},
		"cloudflare dns zone transfers incoming":  {identiferType: "zone", resourceType: "cloudflare_dns_zone_transfers_incoming", testdataFilename: "cloudflare_dns_zone_transfers_incoming"},
		"cloudflare dns zone transfers outgoing":  {identiferType: "zone", resourceType: "cloudflare_dns_zone_transfers_outgoing", testdataFilename: "cloudflare_dns_zone_transfers_outgoing"},
		"cloudflare dns zone transfers peer":      {identiferType: "account", resourceType: "cloudflare_dns_zone_transfers_peer", testdataFilename: "cloudflare_dns_zone_transfers_peer"},
		"cloudflare dns zone transfers tsig":      {identiferType: "account", resourceType: "cloudflare_dns_zone_transfers_tsig", testdataFilename: "cloudflare_dns_zone_transfers_tsig"},
		"cloudflare leaked credential check":      {identiferType: "zone", resourceType: "cloudflare_leaked_credential_check", testdataFilename: "cloudflare_leaked_credential_check"},
		"cloudflare leaked credential check rule": {identiferType: "zone", resourceType: "cloudflare_leaked_credential_check_rule", testdataFilename: "cloudflare_leaked_credential_check_rule"},
		"cloudflare list":                         {identiferType: "account", resourceType: "cloudflare_list", testdataFilename: "cloudflare_list"},
		"cloudflare logpush job":                  {identiferType: "account", resourceType: "cloudflare_logpush_job", testdataFilename: "cloudflare_logpush_job"},
		"cloudflare logpull retention":            {identiferType: "zone", resourceType: "cloudflare_logpull_retention", testdataFilename: "cloudflare_logpull_retention"},
		"cloudflare notification policy":          {identiferType: "account", resourceType: "cloudflare_notification_policy", testdataFilename: "cloudflare_notification_policy"},
		"cloudflare notification policy webhooks": {identiferType: "account", resourceType: "cloudflare_notification_policy_webhooks", testdataFilename: "cloudflare_notification_policy_webhooks"},
		"cloudflare pages project":                {identiferType: "account", resourceType: "cloudflare_pages_project", testdataFilename: "cloudflare_pages_project"},
		"cloudflare page shield policy":           {identiferType: "zone", resourceType: "cloudflare_page_shield_policy", testdataFilename: "cloudflare_page_shield_policy"},
		"cloudflare_r2_bucket":                    {identiferType: "account", resourceType: "cloudflare_r2_bucket", testdataFilename: "cloudflare_r2_bucket"},
		// "cloudflare ruleset (ddos_l7)":                       {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone_ddos_l7"},
		// "cloudflare ruleset (http_log_custom_fields)":        {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone_http_log_custom_fields"},
		// "cloudflare ruleset (http_ratelimit)":                {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone_http_ratelimit"},
		// "cloudflare ruleset (http_request_cache_settings)":   {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_http_request_cache_settings"},
		// "cloudflare ruleset (http_request_firewall_custom)":  {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone_http_request_firewall_custom"},
		// "cloudflare ruleset (http_request_firewall_managed)": {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone_http_request_firewall_managed"},
		// "cloudflare ruleset (http_request_late_transform)":   {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone_http_request_late_transform"},
		// "cloudflare ruleset (http_request_sanitize)":         {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone_http_request_sanitize"},
		// "cloudflare ruleset (no configuration)":              {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone_no_configuration"},
		// "cloudflare ruleset (override remapping = disabled)": {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_override_remapping_disabled"},
		// "cloudflare ruleset (override remapping = enabled)":  {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_override_remapping_enabled"},
		// "cloudflare ruleset (rewrite to empty query string)": {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone_rewrite_to_empty_query_parameter"},
		// "cloudflare ruleset":                                 {identiferType: "zone", resourceType: "cloudflare_ruleset", testdataFilename: "cloudflare_ruleset_zone"},
		"cloudflare stream":                {identiferType: "account", resourceType: "cloudflare_stream", testdataFilename: "cloudflare_stream"},
		"cloudflare stream keys":           {identiferType: "account", resourceType: "cloudflare_stream_key", testdataFilename: "cloudflare_stream_key"},
		"cloudflare stream live input":     {identiferType: "account", resourceType: "cloudflare_stream_live_input", testdataFilename: "cloudflare_stream_live_input"},
		"cloudflare stream webhook":        {identiferType: "account", resourceType: "cloudflare_stream_webhook", testdataFilename: "cloudflare_stream_webhook"},
		"cloudflare snippets":              {identiferType: "zone", resourceType: "cloudflare_snippets", testdataFilename: "cloudflare_snippets"},
		"cloudflare snippet rules":         {identiferType: "zone", resourceType: "cloudflare_snippet_rules", testdataFilename: "cloudflare_snippet_rules"},
		"cloudflare tiered cache":          {identiferType: "zone", resourceType: "cloudflare_tiered_cache", testdataFilename: "cloudflare_tiered_cache"},
		"cloudflare regional hostnames":    {identiferType: "zone", resourceType: "cloudflare_regional_hostname", testdataFilename: "cloudflare_regional_hostname"},
		"cloudflare regional tiered cache": {identiferType: "zone", resourceType: "cloudflare_regional_tiered_cache", testdataFilename: "cloudflare_regional_tiered_cache"},
		// "cloudflare spectrum application":                    {identiferType: "zone", resourceType: "cloudflare_spectrum_application", testdataFilename: "cloudflare_spectrum_application"},
		// "cloudflare teams list":                              {identiferType: "account", resourceType: "cloudflare_teams_list", testdataFilename: "cloudflare_teams_list"},
		// "cloudflare teams location":                          {identiferType: "account", resourceType: "cloudflare_teams_location", testdataFilename: "cloudflare_teams_location"},
		// "cloudflare teams proxy endpoint":                    {identiferType: "account", resourceType: "cloudflare_teams_proxy_endpoint", testdataFilename: "cloudflare_teams_proxy_endpoint"},
		// "cloudflare teams rule":                              {identiferType: "account", resourceType: "cloudflare_teams_rule", testdataFilename: "cloudflare_teams_rule"},
		"cloudflare total tls": {identiferType: "zone", resourceType: "cloudflare_total_tls", testdataFilename: "cloudflare_total_tls"},
		// "cloudflare tunnel":                                  {identiferType: "account", resourceType: "cloudflare_tunnel", testdataFilename: "cloudflare_tunnel"},
		//"cloudflare turnstile_widget":            {identiferType: "account", resourceType: "cloudflare_turnstile_widget", testdataFilename: "cloudflare_turnstile_widget"},
		"cloudflare turnstile_widget_no_domains": {identiferType: "account", resourceType: "cloudflare_turnstile_widget", testdataFilename: "cloudflare_turnstile_widget_no_domains"},
		// "cloudflare url normalization settings":              {identiferType: "zone", resourceType: "cloudflare_url_normalization_settings", testdataFilename: "cloudflare_url_normalization_settings"},
		// "cloudflare turnstile_widget":                        {identiferType: "account", resourceType: "cloudflare_turnstile_widget", testdataFilename: "cloudflare_turnstile_widget"},
		// "cloudflare turnstile_widget_no_domains":             {identiferType: "account", resourceType: "cloudflare_turnstile_widget", testdataFilename: "cloudflare_turnstile_widget_no_domains"},
		"cloudflare url normalization settings": {identiferType: "zone", resourceType: "cloudflare_url_normalization_settings", testdataFilename: "cloudflare_url_normalization_settings"},
		"cloudflare user":                       {identiferType: "account", resourceType: "cloudflare_user", testdataFilename: "cloudflare_user"},
		// "cloudflare user agent blocking rule":                {identiferType: "zone", resourceType: "cloudflare_user_agent_blocking_rule", testdataFilename: "cloudflare_user_agent_blocking_rule"},
		// "cloudflare waiting room":                            {identiferType: "zone", resourceType: "cloudflare_waiting_room", testdataFilename: "cloudflare_waiting_room"},
		// "cloudflare waiting room event":                      {identiferType: "zone", resourceType: "cloudflare_waiting_room_event", testdataFilename: "cloudflare_waiting_room_event"},
		// "cloudflare waiting room rules":                      {identiferType: "zone", resourceType: "cloudflare_waiting_room_rules", testdataFilename: "cloudflare_waiting_room_rules"},
		// "cloudflare waiting room settings":                   {identiferType: "zone", resourceType: "cloudflare_waiting_room_settings", testdataFilename: "cloudflare_waiting_room_settings"},
		"cloudflare web3 hostname": {identiferType: "zone", resourceType: "cloudflare_web3_hostname", testdataFilename: "cloudflare_web3_hostname"},
		// "cloudflare worker route":                            {identiferType: "zone", resourceType: "cloudflare_worker_route", testdataFilename: "cloudflare_worker_route"},
		// "cloudflare workers kv namespace":                    {identiferType: "account", resourceType: "cloudflare_workers_kv_namespace", testdataFilename: "cloudflare_workers_kv_namespace"},
		"cloudflare zone lockdown": {identiferType: "zone", resourceType: "cloudflare_zone_lockdown", testdataFilename: "cloudflare_zone_lockdown"},
		// "cloudflare tiered cache":                            {identiferType: "zone", resourceType: "cloudflare_tiered_cache", testdataFilename: "cloudflare_tiered_cache"},
		//
		// "cloudflare access group (account)": {identiferType: "account", resourceType: "cloudflare_access_group", testdataFilename: "cloudflare_access_group_account"},
		// "cloudflare access group (zone)":    {identiferType: "zone", resourceType: "cloudflare_access_group", testdataFilename: "cloudflare_access_group_zone"},
		// "cloudflare custom certificates":    {identiferType: "zone", resourceType: "cloudflare_custom_certificates", testdataFilename: "cloudflare_custom_certificates"},
		// "cloudflare custom SSL": {identiferType: "zone", resourceType: "cloudflare_custom_ssl", testdataFilename: "cloudflare_custom_ssl"},
		"cloudflare queue":                 {identiferType: "account", resourceType: "cloudflare_queue", testdataFilename: "cloudflare_queue"},
		"cloudflare waiting room":          {identiferType: "zone", resourceType: "cloudflare_waiting_room", testdataFilename: "cloudflare_waiting_room"},
		"cloudflare waiting room settings": {identiferType: "zone", resourceType: "cloudflare_waiting_room_settings", testdataFilename: "cloudflare_waiting_room_settings"},
		// "cloudflare worker cron trigger":    {identiferType: "zone", resourceType: "cloudflare_worker_cron_trigger", testdataFilename: "cloudflare_worker_cron_trigger"},
		"cloudflare workers custom domain":                    {identiferType: "account", resourceType: "cloudflare_workers_custom_domain", testdataFilename: "cloudflare_workers_custom_domain"},
		"cloudflare workers kv namespace":                     {identiferType: "account", resourceType: "cloudflare_workers_kv_namespace", testdataFilename: "cloudflare_workers_kv_namespace"},
		"cloudflare workers for platforms dispatch namespace": {identiferType: "account", resourceType: "cloudflare_workers_for_platforms_dispatch_namespace", testdataFilename: "cloudflare_workers_for_platforms_dispatch_namespace"},
		"cloudflare zone":                                     {identiferType: "zone", resourceType: "cloudflare_zone", testdataFilename: "cloudflare_zone"},
		"cloudflare zone dnssec":                              {identiferType: "zone", resourceType: "cloudflare_zone_dnssec", testdataFilename: "cloudflare_zone_dnssec"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Reset the environment variables used in test to ensure we don't
			// have both present at once.
			viper.Set("zone", "")
			viper.Set("account", "")

			var r *recorder.Recorder
			var err error
			if os.Getenv("OVERWRITE_VCR_CASSETTES") == "true" {
				r, err = recorder.NewAsMode("../../../../testdata/cloudflare/v5/"+tc.testdataFilename, recorder.ModeRecording, http.DefaultTransport)
			} else {
				r, err = recorder.New("../../../../testdata/cloudflare/v5/" + tc.testdataFilename)
			}

			if err != nil {
				log.Fatal(err)
			}
			defer func() {
				err := r.Stop()
				if err != nil {
					log.Fatal(err)
				}
			}()

			r.AddFilter(func(i *cassette.Interaction) error {
				// Sensitive HTTP headers
				delete(i.Request.Headers, "X-Auth-Email")
				delete(i.Request.Headers, "X-Auth-Key")
				delete(i.Request.Headers, "Authorization")

				// HTTP request headers that we don't need to assert against
				delete(i.Request.Headers, "User-Agent")

				// HTTP response headers that we don't need to assert against
				delete(i.Response.Headers, "Cf-Cache-Status")
				delete(i.Response.Headers, "Cf-Ray")
				delete(i.Response.Headers, "Date")
				delete(i.Response.Headers, "Server")
				delete(i.Response.Headers, "Set-Cookie")
				delete(i.Response.Headers, "X-Envoy-Upstream-Service-Time")

				if os.Getenv("CLOUDFLARE_DOMAIN") != "" {
					i.Response.Body = strings.ReplaceAll(i.Response.Body, os.Getenv("CLOUDFLARE_DOMAIN"), "example.com")
				}

				return nil
			})

			output := ""

			if tc.identiferType == "account" {
				viper.Set("account", cloudflareTestAccountID)
				api = cloudflare.NewClient(option.WithHTTPClient(
					&http.Client{
						Transport: r,
					},
				))

				output, _ = executeCommandC(rootCmd, "generate", "--resource-type", tc.resourceType, "--account", cloudflareTestAccountID)
			} else {
				viper.Set("zone", cloudflareTestZoneID)
				api = cloudflare.NewClient(option.WithHTTPClient(
					&http.Client{
						Transport: r,
					},
				))

				output, _ = executeCommandC(rootCmd, "generate", "--resource-type", tc.resourceType, "--zone", cloudflareTestZoneID)
			}

			expected := testDataFile("v5", tc.testdataFilename)
			assert.Equal(t, strings.TrimRight(expected, "\n"), strings.TrimRight(output, "\n"))
		})
	}
}
