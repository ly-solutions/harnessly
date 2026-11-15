// The review screen over a repository's local work: its uncommitted diff, or one commit of it, as «Changes» has them.
import { useMemo } from 'react';
import type { IReviewCommit } from '@/next/features/review/model';
import ReviewScreen from '@/next/features/review/review-screen';
import { localDiffSource } from '@/next/legacy/review-sources';
import { VIEWED_IO } from '@/next/legacy/code-review-api';

interface ILocalReviewProps {
  root: string;
  name: string;
  branch: string | null;
  diff: string;
  truncated: boolean;
  sha?: string;
  title?: string;
  commits?: IReviewCommit[];
}

const LocalReview = ({ root, name, branch, diff, truncated, sha, title, commits }: ILocalReviewProps) => {
  const source = useMemo(
    () => localDiffSource({ root, name, branch, diff, truncated, sha, title, commits }),
    [root, name, branch, diff, truncated, sha, title, commits],
  );
  return <ReviewScreen source={source} viewedIo={VIEWED_IO} />;
};

export default LocalReview;
