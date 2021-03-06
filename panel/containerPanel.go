package panel

import (
	"fmt"
	"os"
	"strings"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/jroimartin/gocui"
	"github.com/skanehira/docui/common"
)

type ContainerList struct {
	*Gui
	name string
	Position
	Containers        []*Container
	Data              map[string]interface{}
	ClosePanelName    string
	Items             Items
	selectedContainer *Container
	filter            string
}

type Container struct {
	ID      string `tag:"ID" len:"min:0.1 max:0.2"`
	Name    string `tag:"NAME" len:"min:0.1 max:0.2"`
	Image   string `tag:"IMAGE" len:"min:0.1 max:0.2"`
	Status  string `tag:"STATUS" len:"min:0.1 max:0.1"`
	Created string `tag:"CREATED" len:"min:0.1 max:0.1"`
	Port    string `tag:"PORT" len:"min:0.1 max:0.2"`
}

func NewContainerList(gui *Gui, name string, x, y, w, h int) *ContainerList {
	return &ContainerList{
		Gui:      gui,
		name:     name,
		Position: Position{x, y, w, h},
		Data:     make(map[string]interface{}),
		Items:    Items{},
	}
}

func (c *ContainerList) Name() string {
	return c.name
}

func (c *ContainerList) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	switch {
	case ch != 0 && mod == 0:
		v.EditWrite(ch)
	case key == gocui.KeySpace:
		v.EditWrite(' ')
	case key == gocui.KeyBackspace || key == gocui.KeyBackspace2:
		v.EditDelete(true)
	case key == gocui.KeyArrowLeft:
		v.MoveCursor(-1, 0, false)
		return
	case key == gocui.KeyArrowRight:
		v.MoveCursor(+1, 0, false)
		return
	}

	c.filter = ReadLine(v, nil)

	if v, err := c.View(c.name); err == nil {
		c.GetContainerList(v)
	}
}

func (c *ContainerList) SetView(g *gocui.Gui) error {
	// set header panel
	if v, err := g.SetView(ContainerListHeaderPanel, c.x, c.y, c.w, c.h); err != nil {
		if err != gocui.ErrUnknownView {
			panic(err)
		}

		v.Wrap = true
		v.Frame = true
		v.Title = v.Name()
		v.FgColor = gocui.AttrBold | gocui.ColorWhite
		common.OutputFormatedHeader(v, &Container{})
	}

	// set scroll panel
	v, err := g.SetView(c.name, c.x, c.y+1, c.w, c.h)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}

		v.Frame = false
		v.Wrap = true
		v.FgColor = gocui.ColorGreen
		v.SelBgColor = gocui.ColorWhite
		v.SelFgColor = gocui.ColorBlack | gocui.AttrBold
		v.SetOrigin(0, 0)
		v.SetCursor(0, 0)

		c.GetContainerList(v)
	}

	c.SetKeyBinding()

	//monitoring container status interval 5s
	go func() {
		for {
			c.Update(func(g *gocui.Gui) error {
				c.Refresh(g, v)
				return nil
			})
			time.Sleep(5 * time.Second)
		}
	}()

	return nil
}

func (c *ContainerList) SetKeyBinding() {
	c.SetKeyBindingToPanel(c.name)

	if err := c.SetKeybinding(c.name, 'j', gocui.ModNone, CursorDown); err != nil {
		panic(err)
	}
	if err := c.SetKeybinding(c.name, 'k', gocui.ModNone, CursorUp); err != nil {
		panic(err)
	}
	if err := c.SetKeybinding(c.name, gocui.KeyEnter, gocui.ModNone, c.DetailContainer); err != nil {
		panic(err)
	}
	if err := c.SetKeybinding(c.name, 'o', gocui.ModNone, c.DetailContainer); err != nil {
		panic(err)
	}
	if err := c.SetKeybinding(c.name, 'd', gocui.ModNone, c.RemoveContainer); err != nil {
		panic(err)
	}
	if err := c.SetKeybinding(c.name, 'u', gocui.ModNone, c.StartContainer); err != nil {
		panic(err)
	}
	if err := c.SetKeybinding(c.name, 's', gocui.ModNone, c.StopContainer); err != nil {
		panic(err)
	}
	if err := c.SetKeybinding(c.name, 'e', gocui.ModNone, c.ExportContainerPanel); err != nil {
		panic(err)
	}
	if err := c.SetKeybinding(c.name, 'c', gocui.ModNone, c.CommitContainerPanel); err != nil {
		panic(err)
	}
	if err := c.SetKeybinding(c.name, 'r', gocui.ModNone, c.RenameContainerPanel); err != nil {
		panic(err)
	}
	if err := c.SetKeybinding(c.name, gocui.KeyCtrlR, gocui.ModNone, c.Refresh); err != nil {
		panic(err)
	}
	if err := c.SetKeybinding(c.name, 'f', gocui.ModNone, c.Filter); err != nil {
		panic(err)
	}
}

func (c *ContainerList) selected() (*Container, error) {
	v, _ := c.View(c.name)
	_, cy := v.Cursor()
	_, oy := v.Origin()

	index := oy + cy
	length := len(c.Containers)

	if index >= length {
		return nil, common.NoContainer
	}
	return c.Containers[cy+oy], nil
}

func (c *ContainerList) DetailContainer(g *gocui.Gui, v *gocui.View) error {
	c.NextPanel = c.name

	selected, err := c.selected()
	if err != nil {
		c.ErrMessage(err.Error(), c.NextPanel)
		return nil
	}

	container, err := c.Docker.InspectContainer(selected.ID)
	if err != nil {
		c.ErrMessage(err.Error(), c.NextPanel)
		return nil
	}

	c.PopupDetailPanel(g, v)

	v, err = g.View(DetailPanel)
	if err != nil {
		panic(err)
	}

	v.Clear()
	v.SetOrigin(0, 0)
	v.SetCursor(0, 0)
	fmt.Fprint(v, common.StructToJson(container))

	return nil
}

func (c *ContainerList) RemoveContainer(g *gocui.Gui, v *gocui.View) error {
	c.NextPanel = c.name

	container, err := c.selected()
	if err != nil {
		c.ErrMessage(err.Error(), c.NextPanel)
		return nil
	}

	c.ConfirmMessage("Are you sure you want to remove this container? (y/n)", func(g *gocui.Gui, v *gocui.View) error {
		defer c.Refresh(g, v)
		defer c.CloseConfirmMessage(g, v)
		options := docker.RemoveContainerOptions{ID: container.ID}

		if err := c.Docker.RemoveContainer(options); err != nil {
			c.ErrMessage(err.Error(), c.NextPanel)
			return nil
		}

		return nil
	})

	return nil
}

func (c *ContainerList) StartContainer(g *gocui.Gui, v *gocui.View) error {
	c.NextPanel = c.name

	container, err := c.selected()
	if err != nil {
		c.ErrMessage(err.Error(), c.NextPanel)
		return nil
	}

	g.Update(func(g *gocui.Gui) error {
		c.StateMessage("container starting...")

		g.Update(func(g *gocui.Gui) error {
			defer c.Refresh(g, v)
			defer c.CloseStateMessage()

			if err := c.Docker.StartContainerWithID(container.ID); err != nil {
				c.ErrMessage(err.Error(), c.NextPanel)
				return nil
			}

			c.SwitchPanel(c.NextPanel)

			return nil
		})

		return nil
	})

	return nil
}

func (c *ContainerList) StopContainer(g *gocui.Gui, v *gocui.View) error {
	c.NextPanel = c.name

	container, err := c.selected()
	if err != nil {
		c.ErrMessage(err.Error(), c.NextPanel)
		return nil
	}

	g.Update(func(g *gocui.Gui) error {
		c.StateMessage("container stopping...")

		g.Update(func(g *gocui.Gui) error {
			defer c.CloseStateMessage()
			defer c.Refresh(g, v)

			if err := c.Docker.StopContainerWithID(container.ID); err != nil {
				c.ErrMessage(err.Error(), c.NextPanel)
				return nil
			}

			c.SwitchPanel(c.NextPanel)

			return nil
		})

		return nil
	})

	return nil
}

func (c *ContainerList) ExportContainerPanel(g *gocui.Gui, v *gocui.View) error {
	c.NextPanel = c.name

	container, err := c.selected()
	if err != nil {
		c.ErrMessage(err.Error(), c.NextPanel)
		return nil
	}

	c.Data = map[string]interface{}{
		"Container": container.Name,
	}

	maxX, maxY := c.Size()
	x := maxX / 8
	y := maxY / 3
	w := maxX - x
	h := y + 10

	c.ClosePanelName = ExportContainerPanel
	c.Items = c.NewExportContainerItems(x, y, w, h)

	handlers := Handlers{
		gocui.KeyEnter: c.ExportContainer,
	}

	NewInput(c.Gui, ExportContainerPanel, x, y, w, h, c.Items, c.Data, handlers)
	return nil
}

func (c *ContainerList) ExportContainer(g *gocui.Gui, v *gocui.View) error {
	path := ReadLine(v, nil)

	if path == "" {
		return nil
	}

	g.Update(func(g *gocui.Gui) error {
		c.ClosePanel(g, v)
		c.StateMessage("container exporting...")

		g.Update(func(g *gocui.Gui) error {
			defer c.CloseStateMessage()

			file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
			if err != nil {
				c.ErrMessage(err.Error(), c.NextPanel)
				return nil
			}
			defer file.Close()

			options := docker.ExportContainerOptions{
				ID:           c.Data["Container"].(string),
				OutputStream: file,
			}

			if err := c.Docker.ExportContainerWithOptions(options); err != nil {
				c.ErrMessage(err.Error(), c.NextPanel)
				return nil
			}

			c.SwitchPanel(c.NextPanel)

			return nil

		})
		return nil
	})

	return nil
}

func (c *ContainerList) CommitContainerPanel(g *gocui.Gui, v *gocui.View) error {
	c.NextPanel = c.name

	container, err := c.selected()
	if err != nil {
		c.ErrMessage(err.Error(), c.NextPanel)
		return nil
	}

	c.Data = map[string]interface{}{
		"Container": container.Name,
	}

	maxX, maxY := c.Size()
	x := maxX / 8
	y := maxY / 3
	w := maxX - x
	h := maxY - y

	c.ClosePanelName = CommitContainerPanel
	c.Items = c.NewCommitContainerItems(x, y, w, h)

	handlers := Handlers{
		gocui.KeyEnter: c.CommitContainer,
	}

	NewInput(c.Gui, CommitContainerPanel, x, y, w, h, c.Items, c.Data, handlers)
	return nil
}

func (c *ContainerList) CommitContainer(g *gocui.Gui, v *gocui.View) error {
	data, err := c.GetItemsToMap(c.Items)
	if err != nil {
		c.ClosePanel(g, v)
		c.ErrMessage(err.Error(), c.NextPanel)
		return nil
	}

	options := docker.CommitContainerOptions{
		Container:  data["Container"],
		Repository: data["Repository"],
		Tag:        data["Tag"],
	}

	g.Update(func(g *gocui.Gui) error {
		c.ClosePanel(g, v)
		c.StateMessage("container committing...")

		g.Update(func(g *gocui.Gui) error {
			defer c.CloseStateMessage()

			if err := c.Docker.CommitContainerWithOptions(options); err != nil {
				c.ErrMessage(err.Error(), c.NextPanel)
				return nil
			}

			c.Panels[ImageListPanel].Refresh(g, v)
			c.SwitchPanel(c.NextPanel)

			return nil

		})

		return nil
	})

	return nil
}

func (c *ContainerList) RenameContainerPanel(g *gocui.Gui, v *gocui.View) error {
	c.NextPanel = c.name

	container, err := c.selected()
	if err != nil {
		c.ErrMessage(err.Error(), c.NextPanel)
		return nil
	}

	c.Data = map[string]interface{}{
		"Container": container.Name,
	}

	maxX, maxY := c.Size()
	x := maxX / 8
	y := maxY/3 + 5
	w := maxX - x
	h := maxY - y

	c.ClosePanelName = RenameContainerPanel
	c.Items = c.NewRenameContainerItems(x, y, w, h)

	handlers := Handlers{
		gocui.KeyEnter: c.RenameContainer,
	}

	NewInput(c.Gui, RenameContainerPanel, x, y, w, h, c.Items, c.Data, handlers)
	return nil
}

func (c *ContainerList) RenameContainer(g *gocui.Gui, v *gocui.View) error {
	data, err := c.GetItemsToMap(c.Items)
	if err != nil {
		c.ClosePanel(g, v)
		c.ErrMessage(err.Error(), c.NextPanel)
		return nil
	}

	options := docker.RenameContainerOptions{
		ID:   data["Container"],
		Name: data["NewName"],
	}

	g.Update(func(g *gocui.Gui) error {
		c.ClosePanel(g, v)
		c.StateMessage("container renaming...")

		g.Update(func(g *gocui.Gui) error {
			defer c.CloseStateMessage()

			if err := c.Docker.RenameContainerWithOptions(options); err != nil {
				c.ErrMessage(err.Error(), c.NextPanel)
				return nil
			}

			c.Refresh(g, v)
			c.SwitchPanel(c.NextPanel)

			return nil

		})

		return nil
	})

	return nil
}

func (c *ContainerList) Refresh(g *gocui.Gui, v *gocui.View) error {
	c.Update(func(g *gocui.Gui) error {
		v, err := c.View(c.name)
		if err != nil {
			panic(err)
		}

		c.GetContainerList(v)

		return nil
	})

	return nil
}

func (c *ContainerList) GetContainerList(v *gocui.View) {
	v.Clear()
	c.Containers = make([]*Container, 0)

	for _, con := range c.Docker.Containers() {
		name := con.Names[0][1:]
		if c.filter != "" {
			if strings.Index(strings.ToLower(name), strings.ToLower(c.filter)) == -1 {
				continue
			}
		}

		id := con.ID[:12]
		image := con.Image
		status := con.Status
		created := ParseDateToString(con.Created)
		port := ParsePortToString(con.Ports)

		container := &Container{
			ID:      id,
			Image:   image,
			Name:    name,
			Status:  status,
			Created: created,
			Port:    port,
		}

		c.Containers = append(c.Containers, container)

		common.OutputFormatedLine(v, container)
	}
}

func (c *ContainerList) Filter(g *gocui.Gui, lv *gocui.View) error {
	c.NextPanel = c.name

	isReset := false
	closePanel := func(g *gocui.Gui, v *gocui.View) error {
		if isReset {
			c.filter = ""
		} else {
			lv.SetCursor(0, 0)
			c.filter = ReadLine(v, nil)
		}
		if v, err := c.View(c.name); err == nil {
			c.GetContainerList(v)
		}

		if err := g.DeleteView(v.Name()); err != nil {
			panic(err)
		}

		g.DeleteKeybindings(v.Name())
		c.SwitchPanel(c.name)
		return nil
	}

	reset := func(g *gocui.Gui, v *gocui.View) error {
		isReset = true
		return closePanel(g, v)
	}

	if err := c.NewFilterPanel(c, reset, closePanel); err != nil {
		panic(err)
	}

	return nil
}

func (c *ContainerList) ClosePanel(g *gocui.Gui, v *gocui.View) error {
	return c.Panels[c.ClosePanelName].(*Input).ClosePanel(g, v)
}

func (c *ContainerList) NewCommitContainerItems(ix, iy, iw, ih int) Items {
	names := []string{
		"Repository",
		"Tag",
		"Container",
	}

	return NewItems(names, ix, iy, iw, ih, 12)
}

func (c *ContainerList) NewExportContainerItems(ix, iy, iw, ih int) Items {
	names := []string{
		"Path",
		"Container",
	}

	return NewItems(names, ix, iy, iw, ih, 10)
}

func (c *ContainerList) NewRenameContainerItems(ix, iy, iw, ih int) Items {
	names := []string{
		"NewName",
		"Container",
	}

	return NewItems(names, ix, iy, iw, ih, 12)
}
