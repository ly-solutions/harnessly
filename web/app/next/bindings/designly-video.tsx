// A video open over the machine: the project of the kind `video` from the library, its state from the video routes
// (kept in stores/video.ts and read again every 3 s while the video is on screen, the runtime's `rev` telling a change)
// and the video's shell. designly-site.tsx renders it for such a project, in the window and in the right pane.
import { useEffect, useState } from 'react';
import VideoView, { type IVideoViewProps } from '@/next/features/designly/video/video-view';
import { fetchVideo } from '@/next/features/designly/video/api';
import { setVideo } from '@/next/stores/video';

export type IDesignlyVideoProps = IVideoViewProps;

/** The video is read again this often while its page is visible, ms. */
const POLL_MS = 3000;

/** Reads the video's state into the store and keeps it fresh; answers why a read failed, null while it is fine. */
const useVideoState = (machine: IVideoViewProps['machine'], id: string): string | null => {
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    let abort: AbortController | null = null;
    let timer: ReturnType<typeof setTimeout> | null = null;
    let live = true;
    const read = () => {
      if (timer) clearTimeout(timer);
      timer = null;
      abort?.abort();
      const now = new AbortController();
      abort = now;
      fetchVideo(machine, id, now.signal).then(
        (state) => {
          if (!live) return;
          setVideo(id, state);
          setError(null);
          schedule();
        },
        (failure: Error) => {
          if (!live || now.signal.aborted) return;
          setError(failure.message);
          schedule();
        },
      );
    };
    const schedule = () => {
      if (live && document.visibilityState === 'visible') timer = setTimeout(read, POLL_MS);
    };
    const visible = () => {
      if (document.visibilityState === 'visible') read();
      else if (timer) clearTimeout(timer);
    };
    read();
    document.addEventListener('visibilitychange', visible);
    return () => {
      live = false;
      abort?.abort();
      if (timer) clearTimeout(timer);
      document.removeEventListener('visibilitychange', visible);
    };
  }, [machine, id]);
  return error;
};

const DesignlyVideo = (props: IDesignlyVideoProps) => {
  const videoError = useVideoState(props.machine, props.id);
  return <VideoView {...props} videoError={videoError} />;
};

export default DesignlyVideo;
