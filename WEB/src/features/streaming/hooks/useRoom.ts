import { useCallback, useEffect, useRef, useState } from "react";
import {
  Room,
  RoomEvent,
  Track,
  type RemoteTrack,
  type RemoteTrackPublication,
  type RemoteParticipant,
} from "livekit-client";
import type { ConnectionState, InputEvent } from "../types/streaming.types";
import {
  DEFAULT_LIVEKIT_URL,
  DEFAULT_ROOM_NAME,
  LIVEKIT_API_KEY,
  LIVEKIT_API_SECRET,
  VIEWER_IDENTITY,
} from "@/shared/constants";

// ─── JWT Token Generation (client-side, dev mode) ───────────────────────

function base64UrlEncode(data: Uint8Array): string {
  let binary = "";
  for (let i = 0; i < data.length; i++) {
    binary += String.fromCharCode(data[i]!);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function strToBase64Url(str: string): string {
  return base64UrlEncode(new TextEncoder().encode(str));
}

async function generateToken(
  apiKey: string,
  apiSecret: string,
  identity: string,
  roomName: string,
): Promise<string> {
  const header = strToBase64Url(JSON.stringify({ alg: "HS256", typ: "JWT" }));

  const now = Math.floor(Date.now() / 1000);
  const payload = strToBase64Url(
    JSON.stringify({
      iss: apiKey,
      sub: identity,
      nbf: now,
      exp: now + 86400,
      iat: now,
      identity,
      name: "Web Viewer",
      video: {
        roomJoin: true,
        room: roomName,
        canPublish: true,
        canPublishData: true,
        canSubscribe: true,
      },
    }),
  );

  const signingInput = `${header}.${payload}`;
  const key = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(apiSecret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  const sig = await crypto.subtle.sign(
    "HMAC",
    key,
    new TextEncoder().encode(signingInput),
  );

  return `${signingInput}.${base64UrlEncode(new Uint8Array(sig))}`;
}

// ─── Hook ────────────────────────────────────────────────────────────────

export function useRoom() {
  const [connectionState, setConnectionState] = useState<ConnectionState>("idle");
  const roomRef = useRef<Room | null>(null);
  const videoRef = useRef<HTMLVideoElement>(null);
  const audioEls = useRef<HTMLAudioElement[]>([]);
  const connectVersionRef = useRef(0);

  // --- Track handlers ---

  const handleTrackSubscribed = useCallback(
    (
      track: RemoteTrack,
      _pub: RemoteTrackPublication,
      participant: RemoteParticipant,
    ) => {
      if (track.kind === Track.Kind.Video) {
        const el = videoRef.current;
        if (el) {
          track.attach(el);
          console.log(`[Stream] Video track from ${participant.identity}`);
        }
      } else if (track.kind === Track.Kind.Audio) {
        const audio = document.createElement("audio");
        audio.autoplay = true;
        audio.style.display = "none";
        document.body.appendChild(audio);
        track.attach(audio);
        audioEls.current.push(audio);
        console.log(`[Stream] Audio track from ${participant.identity}`);
      }
    },
    [],
  );

  const handleTrackUnsubscribed = useCallback(
    (track: RemoteTrack) => {
      track.detach().forEach((el) => {
        if (el instanceof HTMLAudioElement) {
          el.remove();
        }
      });
      audioEls.current = audioEls.current.filter((a) => document.body.contains(a));
    },
    [],
  );

  // --- Attach existing tracks (joined mid-session) ---

  const attachExistingTracks = useCallback(
    (room: Room) => {
      room.remoteParticipants.forEach((participant) => {
        participant.trackPublications.forEach((pub) => {
          if (pub.isSubscribed && pub.track) {
            handleTrackSubscribed(
              pub.track as RemoteTrack,
              pub as RemoteTrackPublication,
              participant,
            );
          }
        });
      });
    },
    [handleTrackSubscribed],
  );

  // --- Connect ---

  const connect = useCallback(async () => {
    if (roomRef.current) return;

    const version = ++connectVersionRef.current;
    const isStale = () => version !== connectVersionRef.current;

    setConnectionState("connecting");

    try {
      const token = await generateToken(
        LIVEKIT_API_KEY,
        LIVEKIT_API_SECRET,
        VIEWER_IDENTITY,
        DEFAULT_ROOM_NAME,
      );

      if (isStale()) return;

      const room = new Room({
        adaptiveStream: true,
        dynacast: true,
      });

      room.on(RoomEvent.Connected, () => {
        if (isStale()) {
          room.disconnect();
          return;
        }
        console.log("[Stream] Connected");
        setConnectionState("connected");
      });

      room.on(RoomEvent.Disconnected, () => {
        if (!isStale()) {
          console.log("[Stream] Disconnected");
          setConnectionState("disconnected");
        }
        roomRef.current = null;
      });

      room.on(RoomEvent.TrackSubscribed, handleTrackSubscribed);
      room.on(RoomEvent.TrackUnsubscribed, handleTrackUnsubscribed);

      room.on(RoomEvent.ParticipantConnected, (p) => {
        console.log(`[Stream] Participant joined: ${p.identity}`);
      });

      room.on(RoomEvent.ParticipantDisconnected, (p) => {
        console.log(`[Stream] Participant left: ${p.identity}`);
      });

      room.on(RoomEvent.DataReceived, (payload, participant) => {
        const text = new TextDecoder().decode(payload);
        console.log(`[Stream] Data from ${participant?.identity}: ${text}`);
      });

      await room.connect(DEFAULT_LIVEKIT_URL, token);

      if (isStale()) {
        room.disconnect();
        return;
      }

      roomRef.current = room;
      attachExistingTracks(room);
    } catch (err) {
      if (!isStale()) {
        console.error("[Stream] Connection failed:", err);
        setConnectionState("error");
      }
      roomRef.current = null;
    }
  }, [handleTrackSubscribed, handleTrackUnsubscribed, attachExistingTracks]);

  // --- Disconnect ---

  const disconnect = useCallback(() => {
    ++connectVersionRef.current; // invalidate any in-flight connect
    const room = roomRef.current;
    if (room) {
      room.disconnect();
      roomRef.current = null;
    }
    audioEls.current.forEach((a) => a.remove());
    audioEls.current = [];
    setConnectionState("idle");
  }, []);

  // --- Send input event via DataChannel ---

  const sendInput = useCallback((event: InputEvent, reliable: boolean) => {
    const room = roomRef.current;
    if (!room?.localParticipant) return;

    const bytes = new TextEncoder().encode(JSON.stringify(event));
    room.localParticipant.publishData(bytes, { reliable });
  }, []);

  // --- Cleanup on unmount ---

  useEffect(() => {
    return () => {
      ++connectVersionRef.current;
      roomRef.current?.disconnect();
      roomRef.current = null;
      audioEls.current.forEach((a) => a.remove());
    };
  }, []);

  return {
    connectionState,
    videoRef,
    connect,
    disconnect,
    sendInput,
  };
}
