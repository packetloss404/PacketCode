import { create } from "zustand";
import { loadFromStorage, saveToStorage, generateId as genId } from "@/lib/storage";
import type { Flight, FlightStatus } from "@/types/flight";
import { useIssueStore } from "@/stores/issueStore";

type FlightState = {
  flights: Flight[];
  activeFlightId: string | null;
};

const DEFAULT_FLIGHT_STATE: FlightState = {
  flights: [],
  activeFlightId: null,
};

const generateFlightId = () => genId("flight");

// Migrate old flights that lack new fields
function migrateFlight(flight: Flight): Flight {
  return {
    ...flight,
    id: flight.id || generateFlightId(),
    title: flight.title || "",
    objective: flight.objective || "",
    status: flight.status || "draft",
    priority: flight.priority || "medium",
    issueIds: flight.issueIds || [],
    linkedSessionIds: flight.linkedSessionIds || [],
    createdAt: flight.createdAt || Date.now(),
    updatedAt: flight.updatedAt || Date.now(),
  };
}

function loadState(): FlightState {
  // Support both new and old localStorage keys for migration
  const newData = loadFromStorage<FlightState>("packetcode:flights", { flights: [], activeFlightId: null });
  if (newData.flights.length > 0) {
    return { ...newData, flights: newData.flights.map(migrateFlight) };
  }
  // Fall back to old key for existing users
  const oldData = loadFromStorage<{ missions?: Flight[]; activeMissionId?: string | null }>("packetcode:missions", {});
  if (oldData.missions && oldData.missions.length > 0) {
    const migrated: FlightState = {
      flights: oldData.missions.map(migrateFlight),
      activeFlightId: oldData.activeMissionId ?? null,
    };
    saveState(migrated);
    return migrated;
  }
  return DEFAULT_FLIGHT_STATE;
}

function saveState(state: FlightState) {
  saveToStorage("packetcode:flights", state);
}

interface FlightStore {
  flights: Flight[];
  activeFlightId: string | null;

  addFlight: (flight: Omit<Flight, "id" | "createdAt" | "updatedAt">) => Flight;
  updateFlight: (id: string, updates: Partial<Flight>) => void;
  deleteFlight: (id: string) => void;
  setActiveFlight: (id: string | null) => void;
  getActiveFlight: () => Flight | null;
  addIssueToFlight: (flightId: string, issueId: string) => void;
  removeIssueFromFlight: (flightId: string, issueId: string) => void;
  linkSessionToFlight: (flightId: string, sessionId: string) => void;
  unlinkSessionFromFlight: (flightId: string, sessionId: string) => void;
  computeFlightStatus: (flightId: string) => FlightStatus;
  getFlightForIssue: (issueId: string) => Flight | null;
}

const initial = loadState();

export const useFlightStore = create<FlightStore>((set, get) => ({
  ...initial,

  addFlight: (flight) => {
    const state = get();
    const newFlight: Flight = {
      ...flight,
      id: generateFlightId(),
      createdAt: Date.now(),
      updatedAt: Date.now(),
    };
    const newState: FlightState = {
      flights: [...state.flights, newFlight],
      activeFlightId: state.activeFlightId,
    };
    set({ flights: newState.flights });
    saveState(newState);
    return newFlight;
  },

  updateFlight: (id, updates) => {
    set((s) => {
      const flights = s.flights.map((f) =>
        f.id === id ? { ...f, ...updates, updatedAt: Date.now() } : f
      );
      saveState({ flights, activeFlightId: s.activeFlightId });
      return { flights };
    });
  },

  deleteFlight: (id) => {
    // Clear flightId from any linked issues before removing the flight
    const flight = get().flights.find((f) => f.id === id);
    if (flight) {
      const { assignToFlight } = useIssueStore.getState();
      for (const issueId of flight.issueIds) {
        assignToFlight(issueId, null);
      }
    }

    set((s) => {
      const flights = s.flights.filter((f) => f.id !== id);
      const activeFlightId = s.activeFlightId === id ? null : s.activeFlightId;
      saveState({ flights, activeFlightId });
      return { flights, activeFlightId };
    });
  },

  setActiveFlight: (id) => {
    set((s) => {
      saveState({ flights: s.flights, activeFlightId: id });
      return { activeFlightId: id };
    });
  },

  getActiveFlight: () => {
    const { flights, activeFlightId } = get();
    if (!activeFlightId) return null;
    return flights.find((f) => f.id === activeFlightId) ?? null;
  },

  addIssueToFlight: (flightId, issueId) => {
    set((s) => {
      const flights = s.flights.map((f) =>
        f.id === flightId && !f.issueIds.includes(issueId)
          ? { ...f, issueIds: [...f.issueIds, issueId], updatedAt: Date.now() }
          : f
      );
      saveState({ flights, activeFlightId: s.activeFlightId });
      return { flights };
    });
  },

  removeIssueFromFlight: (flightId, issueId) => {
    set((s) => {
      const flights = s.flights.map((f) =>
        f.id === flightId
          ? { ...f, issueIds: f.issueIds.filter((id) => id !== issueId), updatedAt: Date.now() }
          : f
      );
      saveState({ flights, activeFlightId: s.activeFlightId });
      return { flights };
    });
  },

  linkSessionToFlight: (flightId, sessionId) => {
    set((s) => {
      const flights = s.flights.map((f) =>
        f.id === flightId && !f.linkedSessionIds.includes(sessionId)
          ? { ...f, linkedSessionIds: [...f.linkedSessionIds, sessionId], updatedAt: Date.now() }
          : f
      );
      saveState({ flights, activeFlightId: s.activeFlightId });
      return { flights };
    });
  },

  unlinkSessionFromFlight: (flightId, sessionId) => {
    set((s) => {
      const flights = s.flights.map((f) =>
        f.id === flightId
          ? { ...f, linkedSessionIds: f.linkedSessionIds.filter((id) => id !== sessionId), updatedAt: Date.now() }
          : f
      );
      saveState({ flights, activeFlightId: s.activeFlightId });
      return { flights };
    });
  },

  computeFlightStatus: (flightId) => {
    const flight = get().flights.find((f) => f.id === flightId);
    if (!flight || flight.issueIds.length === 0) return "draft";
    const { issues } = useIssueStore.getState();
    const linked = issues.filter((i) => flight.issueIds.includes(i.id));
    if (linked.length === 0) return "draft";
    if (linked.some((i) => i.status === "needs_human")) return "needs_human";
    if (linked.some((i) => i.status === "blocked")) return "blocked";
    if (linked.every((i) => i.status === "done")) return "done";
    if (linked.some((i) => i.status === "in_progress" || i.status === "qa")) return "active";
    return "draft";
  },

  getFlightForIssue: (issueId) => {
    return get().flights.find((f) => f.issueIds.includes(issueId)) ?? null;
  },
}));
