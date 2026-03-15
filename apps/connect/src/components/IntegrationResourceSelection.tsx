import { useState } from 'react'
import type { IntegrationProvider, AvailableResource } from '../types'
import type { Action } from '../hooks/useWidget'
import { useAvailableResources } from '../hooks/useAvailableResources'
import { useUpdateConnectionResources } from '../hooks/useUpdateConnectionResources'
import { IntegrationResourceSelectionLogo } from './IntegrationResourceSelectionLogo'
import { Footer } from './Footer'
import { IconButton } from './IconButton'
import { BackIcon, CloseIcon, SearchIcon, CheckIcon, SpinnerIcon } from './icons'

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
  const resourceTypes = integration.resources ?? []
  const [selectedResources, setSelectedResources] = useState<Record<string, string[]>>({})
  const [activeResourceType, setActiveResourceType] = useState<string>(resourceTypes[0]?.type ?? '')
  const [searchQuery, setSearchQuery] = useState('')

  const { data: resources, isLoading, error } = useAvailableResources(
    integration.id ?? '',
    activeResourceType,
    nangoConnectionId
  )

  const updateMutation = useUpdateConnectionResources(connectionId, integration.id ?? '', {
    onSuccess: () => navigate({ type: 'RESOURCE_SELECTION_COMPLETE' }),
    onError: (err) => navigate({ type: 'INTEGRATION_ERROR', error: err }),
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

  const activeTypeInfo = resourceTypes.find((t) => t.type === activeResourceType)

  // Guard clause - should never happen since we only show this screen if there are resources
  if (resourceTypes.length === 0) {
    return null
  }

  return (
    <div className="flex flex-col h-full pb-8">
      <div className="flex items-center shrink-0 gap-3">
        {onBack && (
          <IconButton onClick={onBack}>
            <BackIcon />
          </IconButton>
        )}
        <div className="grow text-xl tracking-tight text-cw-heading cw-mobile:font-semibold cw-desktop:font-bold leading-6">
          Select Resources
        </div>
        <IconButton onClick={onClose}>
          <CloseIcon />
        </IconButton>
      </div>

      <div className="flex items-center gap-3 mt-4 shrink-0">
        <IntegrationResourceSelectionLogo
          providerName={integration.provider ?? ''}
          size="size-10"
          rounded="rounded-lg"
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

      {/* Resource Type Tabs */}
      {resourceTypes.length > 1 && (
        <div className="flex gap-2 mt-4 overflow-x-auto shrink-0 pb-2">
          {resourceTypes.map((type) => (
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

      {/* Search */}
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

      {/* Resource List */}
      <div className="flex-1 overflow-y-auto mt-4 -mx-4 px-4">
        {isLoading && (
          <div className="flex items-center justify-center h-32">
            <SpinnerIcon className="cw-spinner" />
            <span className="ml-2 text-sm text-cw-secondary">Loading...</span>
          </div>
        )}

        {error && (
          <div className="text-center py-8">
            <p className="text-sm text-cw-error">Failed to load resources</p>
            <p className="text-xs text-cw-secondary mt-1">{error}</p>
          </div>
        )}

        {!isLoading && !error && filteredResources.length === 0 && (
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
                <div
                  className={`w-5 h-5 rounded border flex items-center justify-center transition-colors ${
                    isSelected
                      ? 'bg-cw-accent border-cw-accent'
                      : 'border-cw-border bg-white'
                  }`}
                >
                  {isSelected && <CheckIcon className="w-3.5 h-3.5 text-white" />}
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-cw-heading truncate">
                    {resource.name ?? 'Unnamed'}
                  </p>
                  <p className="text-xs text-cw-secondary truncate">
                    {resource.type}
                  </p>
                </div>
              </button>
            )
          })}
        </div>
      </div>

      {/* Footer Actions */}
      <div className="mt-4 pt-4 border-t border-cw-border shrink-0 space-y-3">
        {selectedCount > 0 && (
          <p className="text-xs text-center text-cw-secondary">
            {selectedCount} resource{selectedCount !== 1 ? 's' : ''} selected
          </p>
        )}
        <div className="flex gap-3">
          <button
            onClick={handleSkip}
            disabled={updateMutation.isPending}
            className="flex-1 px-4 py-2.5 text-sm font-medium text-cw-secondary bg-cw-surface border border-cw-border rounded-lg hover:bg-cw-border transition-colors disabled:opacity-50"
          >
            Skip (Full Access)
          </button>
          <button
            onClick={handleSave}
            disabled={updateMutation.isPending || selectedCount === 0}
            className="flex-1 px-4 py-2.5 text-sm font-medium text-white bg-cw-accent rounded-lg hover:bg-cw-accent/90 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {updateMutation.isPending ? (
              <span className="flex items-center justify-center gap-2">
                <SpinnerIcon className="w-4 h-4" />
                Saving...
              </span>
            ) : (
              'Save Selection'
            )}
          </button>
        </div>
      </div>

      <Footer />
    </div>
  )
}
