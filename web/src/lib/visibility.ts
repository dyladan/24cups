import type { Visibility } from '../api'

export const visibilityOptions: { value: Visibility; label: string }[] = [
  { value: 'public', label: 'Everyone' },
  { value: 'groups', label: 'My groups' },
  { value: 'private', label: 'Only me' },
]

export const visibilityHint: Record<Visibility, string> = {
  public: 'Anyone who has rated (or revealed) this coffee sees it. Signed-out visitors see it with spoilers on.',
  groups: 'Only people in a group with you see it, once they’ve rated (or revealed) this coffee.',
  private: 'Only you see it. Its rating still counts toward averages unless you exclude it.',
}
