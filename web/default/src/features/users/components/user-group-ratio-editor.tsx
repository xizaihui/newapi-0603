/*
专属分组倍率编辑器 (Phase 2)

列出该用户已授权的所有分组，每行可填一个倍率覆盖：
- 空：该分组不覆盖（沿用全局或用户组级倍率）
- 有值：该用户在该分组使用此倍率
- 倍率必须 >= 0
*/
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { getUserGroupRatios, setUserGroupRatios } from '../api'

type Props = {
  userId: number
  /** 用户被授权的分组列表 (从 user.group 逗号字段拆出来) */
  groupOptions: string[]
}

export function UserGroupRatioEditor({ userId, groupOptions }: Props) {
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  /** 用户填的字符串值（保留原始输入，提交时再转 number），空字符串 = 不覆盖 */
  const [ratiosInput, setRatiosInput] = useState<Record<string, string>>({})

  // 拉取现有 overrides
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    getUserGroupRatios(userId)
      .then((res) => {
        if (cancelled) return
        const data = res.data || {}
        const next: Record<string, string> = {}
        for (const [g, r] of Object.entries(data)) {
          next[g] = String(r)
        }
        setRatiosInput(next)
      })
      .catch(() => {
        // 静默：非管理员调不到
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [userId])

  const handleChange = (group: string, value: string) => {
    setRatiosInput((prev) => ({ ...prev, [group]: value }))
  }

  const handleSave = async () => {
    // 校验 + 构造 payload
    const payload: Record<string, number> = {}
    for (const [g, str] of Object.entries(ratiosInput)) {
      const trimmed = str.trim()
      if (trimmed === '') continue // 空 = 不覆盖
      const n = Number(trimmed)
      if (!isFinite(n) || n < 0) {
        toast.error(`分组 "${g}" 的倍率无效：${trimmed}`)
        return
      }
      const dotIdx = trimmed.indexOf('.')
      if (dotIdx >= 0 && trimmed.length - dotIdx - 1 > 4) {
        toast.error(`分组 "${g}" 的倍率最多支持 4 位小数：${trimmed}`)
        return
      }
      payload[g] = n
    }
    setSaving(true)
    try {
      const res = await setUserGroupRatios(userId, payload)
      if (res.success) {
        toast.success('专属分组倍率已保存')
      } else {
        toast.error(res.message || '保存失败')
      }
    } catch (e: unknown) {
      toast.error((e as Error).message || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  if (groupOptions.length === 0) {
    return (
      <Card className='border-dashed'>
        <CardHeader className='pb-2'>
          <CardTitle className='text-sm'>专属分组倍率</CardTitle>
          <CardDescription className='text-xs'>
            请先在上方为该用户授权至少一个分组，再回来配置专属倍率。
          </CardDescription>
        </CardHeader>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader className='pb-3'>
        <CardTitle className='text-sm'>专属分组倍率</CardTitle>
        <CardDescription className='text-xs'>
          为该用户单独设置分组倍率，留空则沿用「全局 / 用户组」倍率。值越小越便宜（例如 0.8 = 八折）。优先级：本设置 &gt; 用户组倍率 &gt; 全局分组倍率。
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-3'>
        {loading ? (
          <p className='text-muted-foreground text-xs'>加载中...</p>
        ) : (
          groupOptions.map((g) => (
            <div key={g} className='flex items-center gap-2'>
              <Label className='w-32 truncate text-xs' title={g}>
                {g}
              </Label>
              <Input
                type='number'
                min='0'
                step='0.0001'
                placeholder='留空 = 不覆盖'
                value={ratiosInput[g] ?? ''}
                onChange={(e) => handleChange(g, e.target.value)}
                className='h-8 text-sm'
              />
            </div>
          ))
        )}
        <div className='flex justify-end pt-2'>
          <Button
            type='button'
            size='sm'
            onClick={handleSave}
            disabled={loading || saving}
          >
            {saving ? '保存中...' : '保存专属倍率'}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
