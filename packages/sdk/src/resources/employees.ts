import { BaseResource } from "./base.js";

export class EmployeesResource extends BaseResource {
  list(query?: { limit?: number; cursor?: string; status?: string }) {
    return this.client.GET("/v1/employees", { params: { query } });
  }

  get(id: string) {
    return this.client.GET("/v1/employees/{id}", {
      params: { path: { id } },
    });
  }

  listSkills(id: string) {
    return this.client.GET("/v1/employees/{id}/skills", {
      params: { path: { id } },
    });
  }

  attachSkill(id: string, body: { skill_id: string; pinned_version_id?: string }) {
    return this.client.POST("/v1/employees/{id}/skills", {
      params: { path: { id } },
      body,
    });
  }

  detachSkill(id: string, skillID: string) {
    return this.client.DELETE("/v1/employees/{id}/skills/{skillID}", {
      params: { path: { id, skillID } },
    });
  }

  listSpecialists(id: string) {
    return this.client.GET("/v1/employees/{id}/specialists", {
      params: { path: { id } },
    });
  }

  enableSpecialist(id: string, slug: string) {
    return this.client.POST("/v1/employees/{id}/specialists/{slug}", {
      params: { path: { id, slug } },
    });
  }

  disableSpecialist(id: string, slug: string) {
    return this.client.DELETE("/v1/employees/{id}/specialists/{slug}", {
      params: { path: { id, slug } },
    });
  }
}
