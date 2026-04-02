import { BaseResource } from "./base.js";

export class ReportingResource extends BaseResource {
  get(query?: {
    group_by?: string;
    date_part?: string;
    start_date?: string;
    end_date?: string;
    model?: string;
    provider_id?: string;
    credential_id?: string;
    user_id?: string;
    tags?: string;
  }) {
    return this.client.GET("/v1/reporting", { params: { query } });
  }
}
