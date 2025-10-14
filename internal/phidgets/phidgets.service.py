# // Copyright (C) 2025 Josh Simonot
# //
# // This program is free software: you can redistribute it and/or modify
# // it under the terms of the GNU General Public License as published by
# // the Free Software Foundation, either version 3 of the License, or
# // (at your option) any later version.
# //
# // This program is distributed in the hope that it will be useful,
# // but WITHOUT ANY WARRANTY; without even the implied warranty of
# // MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# // GNU General Public License for more details.
# //
# // You should have received a copy of the GNU General Public License
# // along with this program.  If not, see <https://www.gnu.org/licenses/>.

from aiohttp import web

from Phidget22.Phidget import *
from Phidget22.PhidgetException import *
from Phidget22.Devices.DigitalOutput import *
from Phidget22.Devices.VoltageOutput import *
from Phidget22.Devices.DigitalInput import *

import sys
import json
import aiohttp
import asyncio
import traceback

## active phidget channels
named_phidgets = {}
webhook_registry = {}  # name -> list of webhook URLs


class NamedPhidget:
    def __init__(self, name, phidget):
        self.phidget = phidget
        self.name = name
    
    def toSerializable(self):
        if isinstance(self.phidget, DigitalOutput):
            return {
                "name": self.name,
                "type": "DigitalOutput",
                "state": self.phidget.getState(),
            }
        if isinstance(self.phidget, VoltageOutput): 
            return {
                "name": self.name,
                "type": "VoltageOutput",
                "voltage": self.phidget.getVoltage(),
            }
        if isinstance(self.phidget, DigitalInput):
            return {
                "name": self.name,
                "type": "DigitalInput",
                "state": self.phidget.getState(),
            }


def name_from_phidget(phidget):
    for name, wrapper in named_phidgets.items():
        if phidget == wrapper.phidget:
            return name


def onAttach(self):
    print(f"Attached: {self}")


def onDetach(self):
    print(f"Detached: {self}")
    del named_phidgets[name_from_phidget(self)]


def onError(self, code, description):
    print("Device: " + str(self))
    print("Code: " + ErrorEventCode.getName(code))
    print("Description: " + str(description))
    print("----------")


async def fire_webhooks(name, state):
    urls = webhook_registry.get(name, [])
    async with aiohttp.ClientSession() as session:
        for url in urls:
            try:
                await session.post(url, json={"name": name, "state": state})
            except Exception as e:
                print(f"Webhook {url} failed: {e}")


def onInputChange(self, state):
    name = name_from_phidget(self)
    print(f"DigitalInput {name} state changed: {state}")
    asyncio.create_task(fire_webhooks(name, state))


async def set_digital_output(request):
    try:
        data = await request.json()

        if "name" not in data or "target_state" not in data:
            return web.Response(status=400, text="requires name (str) and target_state (bool)")
        
        name = data["name"]
        if not isinstance(name, str):
            return web.Response(status=400, text="name must be a string")

        target_state = data["target_state"]
        if not isinstance(target_state, bool):
            return web.Response(status=400, text="target_state must be a boolean")

        channel = data.get("channel", -2)
        if not isinstance(channel, int):
            return web.Response(status=400, text="channel must be an integer")

        hub_port = data.get("hub_port", -2)
        if not isinstance(hub_port, int):
            return web.Response(status=400, text="hub_port must be an integer")
    
    except:
        return web.Response(status=400, text="bad request")

    phiwrap = named_phidgets.get(name)
    if not phiwrap:
        if channel == -1 or hub_port == -1:
            return web.Response(status=400, text="name not found; channel and hub_port must be set")
        try:
            do = DigitalOutput()
            do.setChannel(channel)
            do.setHubPort(hub_port)
            
            do.setOnAttachHandler(onAttach)
            do.setOnDetachHandler(onDetach)
            do.setOnErrorHandler(onError)
            do.openWaitForAttachment(5000)

            phiwrap = NamedPhidget(name, do)
            named_phidgets[name] = phiwrap

        except PhidgetException as ex:
            traceback.print_exc()
            return web.Response(status=500, text=str(ex))

    phiwrap.phidget.setState(target_state)
    return web.Response(status=200, text="ACK")


async def set_voltage_output(request):
    try:
        data = await request.json()

        if "name" not in data or "target_state" not in data:
            return web.Response(status=400, text="requires name (str) and target_state (float)")
        
        name = data["name"]
        if not isinstance(name, str):
            return web.Response(status=400, text="name must be a string")

        target_state = data["target_state"]
        if type(target_state) not in (int, float):
            return web.Response(status=400, text="target_state must be an int or float")

        if target_state > 10.0 or target_state < -10.0:
            return web.Response(status=400, text="target_state must be +/- 10V")

        channel = data.get("channel", -2)
        if not isinstance(channel, int):
            return web.Response(status=400, text="channel must be an integer")

        hub_port = data.get("hub_port", -2)
        if not isinstance(hub_port, int):
            return web.Response(status=400, text="hub_port must be an integer")
    
    except:
        return web.Response(status=400, text="bad request")

    phiwrap = named_phidgets.get(name)
    if not phiwrap:
        if channel == -1 or hub_port == -1:
            return web.Response(status=400, text="name not found; channel and hub_port must be set")
        try:
            vo = VoltageOutput()
            vo.setChannel(channel)
            vo.setHubPort(hub_port)
            
            vo.setOnAttachHandler(onAttach)
            vo.setOnDetachHandler(onDetach)
            vo.setOnErrorHandler(onError)
            vo.openWaitForAttachment(5000)

            phiwrap = NamedPhidget(name, vo)
            named_phidgets[name] = phiwrap

        except PhidgetException as ex:
            traceback.print_exc()
            return web.Response(status=500, text=str(ex))

    phiwrap.phidget.setVoltage(target_state)
    return web.Response(status=200, text="ACK")


async def open_digital_input(request):
    try:
        data = await request.json()
        name = data["name"]
        channel = data["channel"]
        hub_port = data["hub_port"]
        webhook_url = data.get("webhook")
    except:
        return web.Response(status=400, text="requires name (str), channel (int), hub_port (int)")

    phiwrap = named_phidgets.get(name)
    if not phiwrap:
        try:
            di = DigitalInput()
            di.setChannel(channel)
            di.setHubPort(hub_port)

            di.setOnAttachHandler(onAttach)
            di.setOnDetachHandler(onDetach)
            di.setOnErrorHandler(onError)
            di.setOnStateChangeHandler(onInputChange)

            di.openWaitForAttachment(5000)

            phiwrap = NamedPhidget(name, di)
            named_phidgets[name] = phiwrap
        except PhidgetException as ex:
            traceback.print_exc()
            return web.Response(status=500, text=str(ex))

    if webhook_url:
        webhook_registry.setdefault(name, []).append(webhook_url)

    return web.Response(status=200, text="ACK")


async def close_phidget_channel(request):
    try:
        data = await request.json()

        if "name" not in data:
            return web.Response(status=400, text="requires name (str)")

        name = data["name"]
        if not isinstance(name, str):
            return web.Response(status=400, text="name must be a string")

    except:
        return web.Response(status=400, text="bad request")

    if name not in named_phidgets:
        return web.Response(status=400, text="no phidget by that name")

    try:
        named_phidgets[name].phidget.close()
        del named_phidgets[name]
        return web.Response(status=200, text="ACK")

    except Exception as ex:
        traceback.print_exc()
        raise web.HTTPBadRequest(reason=str(ex))


async def get_phidgets_state(request):
    serializables = [phiwrap.toSerializable() for phiwrap in named_phidgets.values()]
    out = """
<!DOCTYPE html>
<html>
<head>
    <title>Phidgets State</title>
</head>
<body>
    <p>[OK] /services/actuators/phidgets<p>
    <pre>
""" + json.dumps(serializables, indent=4) + """
    </pre>
</body>
</html>"""
    return web.Response(status=200, text=out, content_type="text/html")


async def root_redirect(request):
    raise web.HTTPFound("/phidgets/state")


app = web.Application()
app.add_routes([
    web.post('/phidgets/digital_out', set_digital_output),
    web.post('/phidgets/voltage_out', set_voltage_output),
    web.post('/phidgets/digital_in', open_digital_input),
    web.post('/phidgets/close', close_phidget_channel),
    web.get('/phidgets/state', get_phidgets_state),
    web.get('/', root_redirect),
])


if __name__ == "__main__":
    port_arg = sys.argv[1] if len(sys.argv) > 1 else "4002"
    if port_arg.startswith(":"):
        port_arg = port_arg[1:]

    port = int(port_arg)
    print(f"Starting server on 0.0.0.0:{port}")
    web.run_app(app, host="0.0.0.0", port=port)
