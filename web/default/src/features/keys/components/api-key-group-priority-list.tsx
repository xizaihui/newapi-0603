/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { ChevronDown, ChevronUp, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { type ApiKeyGroupOption } from './api-key-group-combobox'

type ApiKeyGroupPriorityListProps = {
  /** Selected group values in priority order (index 0 = highest priority). */
  value: string[]
  options: ApiKeyGroupOption[]
  onChange: (next: string[]) => void
  disabled?: boolean
}

/**
 * feat4: priority ordering for multi-group keys. The order shown here IS the
 * runtime priority — each request tries the groups top-to-bottom, only failing
 * over to the next once the current group's channels are exhausted. A group that
 * fails over enters a short cooldown and is skipped until it recovers. Reordering
 * rewrites the comma-separated `group` value the backend iterates in order.
 */
export function ApiKeyGroupPriorityList({
  value,
  options,
  onChange,
  disabled,
}: ApiKeyGroupPriorityListProps) {
  const { t } = useTranslation()

  // Priority only matters with 2+ concrete groups. ("auto" is exclusive, so it
  // never appears here as part of a multi-selection.)
  if (!value || value.length < 2) return null

  const optionFor = (v: string) => options.find((o) => o.value === v)
  const labelFor = (v: string) => optionFor(v)?.label ?? v
  const ratioFor = (v: string) => optionFor(v)?.ratio

  const move = (from: number, to: number) => {
    if (to < 0 || to >= value.length) return
    const next = [...value]
    const [item] = next.splice(from, 1)
    next.splice(to, 0, item)
    onChange(next)
  }

  const remove = (index: number) => {
    onChange(value.filter((_, i) => i !== index))
  }

  return (
    <div className='bg-muted/30 space-y-2 rounded-lg border p-3'>
      <div className='flex items-center justify-between'>
        <span className='text-foreground text-xs font-medium'>
          {t('Priority order')}
        </span>
        <span className='text-muted-foreground text-[11px]'>
          {t('Top = tried first')}
        </span>
      </div>
      <ol className='space-y-1.5'>
        {value.map((group, index) => {
          const ratio = ratioFor(group)
          const showRatio =
            ratio !== undefined && ratio !== null && ratio !== ''
          return (
            <li
              key={group}
              className='bg-background flex items-center gap-2 rounded-md border px-2 py-1.5'
            >
              <span className='bg-primary/10 text-primary inline-flex size-5 shrink-0 items-center justify-center rounded-full text-[11px] font-semibold tabular-nums'>
                {index + 1}
              </span>
              <span className='min-w-0 flex-1 truncate text-sm font-medium'>
                {labelFor(group)}
              </span>
              {showRatio && (
                <Badge
                  variant='outline'
                  className='shrink-0 text-[10px] sm:text-xs'
                >
                  {ratio}x
                </Badge>
              )}
              <div className='flex shrink-0 items-center'>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon'
                  className='size-7'
                  disabled={disabled || index === 0}
                  onClick={() => move(index, index - 1)}
                  aria-label={t('Move up')}
                >
                  <ChevronUp className='size-4' />
                </Button>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon'
                  className='size-7'
                  disabled={disabled || index === value.length - 1}
                  onClick={() => move(index, index + 1)}
                  aria-label={t('Move down')}
                >
                  <ChevronDown className='size-4' />
                </Button>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon'
                  className='text-muted-foreground hover:text-destructive size-7'
                  disabled={disabled}
                  onClick={() => remove(index)}
                  aria-label={t('Remove')}
                >
                  <X className='size-4' />
                </Button>
              </div>
            </li>
          )
        })}
      </ol>
    </div>
  )
}
