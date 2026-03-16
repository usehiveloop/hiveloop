import { useState, useMemo } from 'react'
import { AnimatePresence } from 'motion/react'
import type { IntegrationProvider, IntegrationResource, AvailableResource } from '../types'
import type { Action } from '../hooks/useWidget'
import { useAvailableResources } from '../hooks/useAvailableResources'
import { useIntegrationProviders } from '../hooks/useIntegrationProviders'
import { useUpdateConnectionResources } from '../hooks/useUpdateConnectionResources'
import { useParentEvents } from '../hooks/useParentEvents'
import { IntegrationResourceSelectionLogo } from './IntegrationResourceSelectionLogo'
import { FadeView } from './AnimatedView'
import { Error as ErrorScreen } from './Error'
import { Button } from './Button'
import { ResourceIcon } from './ResourceIcon'
import { Footer } from './Footer'
import { IconButton } from './IconButton'
import { BackIcon, CloseIcon, SearchIcon, SpinnerIcon } from './icons'

interface Props {
  integration: IntegrationProvider
  connectionId: string
  nangoConnectionId: string
  navigate: (action: Action) => void
  onBack?: () => void
  onClose: () => void
}

export function IntegrationResourceSelection({
  integration,
  connectionId,
  nangoConnectionId,
  navigate,
  onBack,
  onClose,
}: Props) {
  const { sendToParent } = useParentEvents()
  const resourceTypes = integration.resources ?? []

  const { data: integrations } = useIntegrationProviders()
  const freshIntegration = useMemo(
    () => integrations?.find((integration) => integration.id === integration.id),
    [integrations, integration.id]
  )

  const [selectedResources, setSelectedResources] = useState<Record<string, string[]>>(
    () => {
      const src = freshIntegration?.selected_resources ?? integration.selected_resources
      const initial: Record<string, string[]> = {}
      if (src) {
        for (const [key, ids] of Object.entries(src)) {
          if (Array.isArray(ids)) {
            initial[key] = [...ids]
          }
        }
      }
      return initial
    }
  )
  const [activeResourceType, setActiveResourceType] = useState<string>(resourceTypes[0]?.type ?? '')
  const [searchQuery, setSearchQuery] = useState('')

  const { data: resources, isLoading, error, refetch } = useAvailableResources(
    integration.id ?? '',
    activeResourceType,
    nangoConnectionId
  )

  const updateMutation = useUpdateConnectionResources(connectionId, integration.id ?? '', {
    onSuccess: () => {
      sendToParent({
        type: 'resource_selection',
        payload: {
          integrationId: integration.unique_key ?? '',
          resources: selectedResources,
        },
      })
      navigate({ type: 'RESOURCE_SELECTION_COMPLETE' })
    },
    onError: () => {},
  })

  const toggleResource = (resourceId: string) => {
    if (!activeResourceType) return
    setSelectedResources((prev) => {
      const current = prev[activeResourceType] ?? []
      const updated = current.includes(resourceId)
        ? current.filter((id) => id !== resourceId)
        : [...current, resourceId]
      return { ...prev, [activeResourceType]: updated }
    })
  }

  const handleSave = () => {
    updateMutation.mutate(selectedResources)
  }

  const handleSkip = () => {
    navigate({ type: 'RESOURCE_SELECTION_SKIP' })
  }

  const filteredResources = resources?.resources?.filter((r: AvailableResource) =>
    (r.name ?? '').toLowerCase().includes(searchQuery.toLowerCase())
  ) ?? []

  const selectedCount = Object.values(selectedResources).flat().length

  const activeTypeInfo = resourceTypes.find((t: IntegrationResource) => t.type === activeResourceType)

  if (resourceTypes.length === 0) {
    return null
  }

  return (
    <AnimatePresence mode="wait">
      {error ? (
        <FadeView viewKey="fetch-error">
          <ErrorScreen
            title="Failed to load resources"
            message={error.message}
            retryLabel="Try again"
            onRetry={() => refetch()}
            onCancel={onBack ?? onClose}
          />
        </FadeView>
      ) : updateMutation.isError ? (
        <FadeView viewKey="save-error">
          <ErrorScreen
            title="Failed to save selection"
            message={updateMutation.error instanceof Error ? updateMutation.error.message : 'Something went wrong while saving. Please try again.'}
            retryLabel="Try again"
            onRetry={() => {
              updateMutation.reset()
              handleSave()
            }}
            onCancel={() => updateMutation.reset()}
          />
        </FadeView>
      ) : (
        <FadeView viewKey="content">
          <div className="flex flex-col h-full pb-8">
          <div className="flex items-center shrink-0 gap-3">
            {onBack && (
              <IconButton onClick={onBack}>
                <BackIcon />
              </IconButton>
            )}
            <div className="grow text-xl tracking-tight text-cw-heading cw-mobile:font-semibold cw-desktop:font-bold leading-6">
              Select {activeResourceType}s
            </div>
            <IconButton onClick={onClose}>
              <CloseIcon />
            </IconButton>
          </div>

          <div className="flex items-center gap-3 mt-4 shrink-0">
            <IntegrationResourceSelectionLogo
              providerName={integration.provider ?? ''}
              className="size-10 rounded-lg"
            />
            <div className="flex flex-col">
              <span className="text-sm font-medium text-cw-heading">
                {integration.display_name ?? integration.provider}
              </span>
              <span className="text-xs text-cw-secondary">
                Choose what the AI can access
              </span>
            </div>
          </div>

          {resourceTypes.length > 1 && (
            <div className="flex gap-2 mt-4 overflow-x-auto shrink-0 pb-2">
              {resourceTypes.map((type: IntegrationResource) => (
                <button
                  key={type.type ?? 'unknown'}
                  onClick={() => setActiveResourceType(type.type ?? '')}
                  className={`px-3 py-1.5 text-sm rounded-full whitespace-nowrap transition-colors ${
                    activeResourceType === type.type
                      ? 'bg-cw-accent text-white'
                      : 'bg-cw-surface text-cw-secondary hover:bg-cw-border'
                  }`}
                >
                  {type.display_name ?? type.type}
                  {type.type && selectedResources[type.type]?.length > 0 && (
                    <span className="ml-1.5 px-1.5 py-0.5 text-xs bg-white/20 rounded-full">
                      {selectedResources[type.type].length}
                    </span>
                  )}
                </button>
              ))}
            </div>
          )}

          <div className="relative mt-4 shrink-0">
            <SearchIcon className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-cw-secondary" />
            <input
              type="text"
              placeholder={`Search ${(activeTypeInfo?.display_name ?? activeTypeInfo?.type ?? 'resources').toLowerCase()}...`}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full pl-9 pr-4 py-2 text-sm bg-cw-surface border border-cw-border rounded-lg text-cw-heading placeholder:text-cw-secondary focus:outline-none focus:ring-2 focus:ring-cw-accent"
            />
          </div>

          <div className="flex-1 overflow-y-auto mt-4 -mx-4 px-4">
            {isLoading && (
              <div className="flex items-center justify-center h-32">
                <SpinnerIcon className="cw-spinner" />
                <span className="ml-2 text-sm text-cw-secondary">Loading...</span>
              </div>
            )}

            {!isLoading && filteredResources.length === 0 && (
              <div className="text-center py-8">
                <p className="text-sm text-cw-secondary">
                  {searchQuery ? 'No matching resources found' : `No ${(activeTypeInfo?.display_name ?? activeTypeInfo?.type ?? 'resources').toLowerCase()} available`}
                </p>
              </div>
            )}

            <div className="space-y-2">
              {filteredResources.map((resource: AvailableResource) => {
                if (!resource.id) return null
                const isSelected = activeResourceType ? selectedResources[activeResourceType]?.includes(resource.id) : false
                return (
                  <button
                    key={resource.id}
                    onClick={() => resource.id && toggleResource(resource.id)}
                    className={`w-full flex items-center gap-3 p-3 rounded-lg border transition-all text-left ${
                      isSelected
                        ? 'border-cw-accent bg-cw-accent/5'
                        : 'border-cw-border bg-cw-surface hover:border-cw-accent/50'
                    }`}
                  >
                    <ResourceIcon iconName={activeTypeInfo?.icon} size={14} className="size-7 shrink-0" />
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-cw-heading truncate">
                        {resource.name ?? 'Unnamed'}
                      </p>
                      <p className="text-xs text-cw-secondary truncate">
                        {resource.type}
                      </p>
                    </div>
                    <div
                      className={`relative w-9 h-5 rounded-full shrink-0 transition-colors ${
                        isSelected ? 'bg-cw-accent' : 'bg-cw-border'
                      }`}
                    >
                      <div
                        className={`absolute top-0.5 h-4 w-4 rounded-full bg-white shadow-sm transition-transform ${
                          isSelected ? 'translate-x-4' : 'translate-x-0.5'
                        }`}
                      />
                    </div>
                  </button>
                )
              })}
            </div>
          </div>

          <div className="mt-4 pt-4 w-full shrink-0 space-y-2">
            <Button
              onClick={handleSave}
              disabled={selectedCount === 0}
              loading={updateMutation.isPending}
              className='w-full'
            >
              Save Selection
            </Button>
            <button
              onClick={handleSkip}
              disabled={updateMutation.isPending}
              className="w-full py-2 text-sm text-cw-secondary hover:text-cw-heading transition-colors disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer"
            >
              Skip
            </button>
          </div>

          <Footer />
          </div>
        </FadeView>
      )}
    </AnimatePresence>
  )
}
