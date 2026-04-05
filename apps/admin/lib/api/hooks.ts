import createQueryHooks from "openapi-react-query"
import { api } from "./client"

export const $api = createQueryHooks(api)
