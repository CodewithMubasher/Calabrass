import { router } from "@/router"

let routerInstance: typeof router | null = null

export function setRouter(r: typeof router) {
  routerInstance = r
}

export function navigate(options: Parameters<typeof router.navigate>[0]) {
  routerInstance?.navigate(options)
}

export function getRouter() {
  return routerInstance
}