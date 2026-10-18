import React, {useCallback, useEffect, useState} from 'react';

import user from "../api/resources/user";
import Login from "./views/Login";
import {Navigate, Route, Routes} from "react-router";
import Controls from "./views/Controls";
import {BrowserRouter, Outlet} from "react-router-dom";
import Logs from "./views/Logs";
import Saves from "./views/Saves/Saves";
import Layout from "./components/Layout";
import server from "../api/resources/server";
import Mods from "./views/Mods/Mods";
import UserManagement from "./views/UserManagement/UserManagment";
import ServerSettings from "./views/ServerSettings";
import GameSettings from "./views/GameSettings";
import Console from "./views/Console";
import Help from "./views/Help";
import socket from "../api/socket";
import {Flash} from "./components/Flash";


const ProtectedRoute = ({isAuthenticated}) => {
    if (!isAuthenticated) return <Navigate to="/login" state={{from: window.location.pathname}} />;
    return <Outlet/>;
};

const App = () => {

    const [isAuthenticated, setIsAuthenticated] = useState(false);
    const [serverStatus, setServerStatus] = useState(null);

    const handleAuthenticationStatus = useCallback(async (status) => {
        if (status?.username) {
            const current = await server.status();
            setServerStatus(current);
            setIsAuthenticated(true);

        }
    },[]);

    const handleLogout = useCallback(async () => {
        const loggedOut = await user.logout();
        if (loggedOut) {
            setIsAuthenticated(false);
        }
    }, []);

    const refreshStatus = useCallback(async () => {
        setServerStatus(await server.status());
    }, []);

    useEffect(() => {
        if (!isAuthenticated) return;
        let disposed = false;
        let timer;
        let polling = false;
        const controller = new AbortController();
        const update = status => setServerStatus(JSON.parse(status));
        const poll = async () => {
            if (disposed || polling) return;
            clearTimeout(timer);
            polling = true;
            try {
                const status = await server.status({signal: controller.signal, silent: true});
                if (!disposed) setServerStatus(status);
            } catch {
                // Keep the last status during transient failures; retry on the next poll.
            } finally {
                polling = false;
                if (!disposed) timer = setTimeout(poll, 5000);
            }
        };
        socket.on('server_status', update);
        socket.on('connected', poll);
        socket.emit('server status subscribe');
        poll();
        return () => {
            disposed = true;
            clearTimeout(timer);
            controller.abort();
            socket.off('server_status', update);
            socket.off('connected', poll);
            socket.emit('server status unsubscribe');
        };
    }, [isAuthenticated]);

    return (
        <BrowserRouter>
            <Routes>
                <Route path="login" element={<Login handleLogin={handleAuthenticationStatus}/>}/>

                {/* route with only `element` will cause the proper children to be place in `<Outlet/>` */}
                <Route element={<ProtectedRoute isAuthenticated={isAuthenticated}/> }>
                    <Route element={<Layout handleLogout={handleLogout} serverStatus={serverStatus} />}>
                        <Route index element={<Controls serverStatus={serverStatus} refreshStatus={refreshStatus}/>}/>
                        <Route path="saves" element={<Saves serverStatus={serverStatus}/>}/>
                        <Route path="mods" element={<Mods serverStatus={serverStatus}/>}/>
                        <Route path="server-settings" element={<ServerSettings serverStatus={serverStatus}/>}/>
                        <Route path="game-settings" element={<GameSettings serverStatus={serverStatus}/>}/>
                        <Route path="console" element={<Console serverStatus={serverStatus}/>}/>
                        <Route path="logs" element={<Logs serverStatus={serverStatus}/>}/>
                        <Route path="user-management" element={<UserManagement serverStatus={serverStatus}/>}/>
                        <Route path="help" element={<Help serverStatus={serverStatus}/>}/>
                    </Route>
                </Route>
            </Routes>
        </BrowserRouter>
    );
}

export default App;
